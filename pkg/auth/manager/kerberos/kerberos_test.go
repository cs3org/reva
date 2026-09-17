// Copyright 2018-2026 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package kerberos

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

// ── test doubles ─────────────────────────────────────────────────────────────

// stubValidator stands in for a KDC. Producing a genuine AP-REQ needs a live
// realm, which is what the integration tests are for; what is worth testing
// here is everything the manager does around the validation.
type stubValidator struct {
	ticket *Ticket
	err    error
	calls  int
	// gotToken records the decoded bytes, so a test can check the manager
	// decoded what it was given rather than passing something else on.
	gotToken []byte
}

func (v *stubValidator) Validate(_ context.Context, token []byte) (*Ticket, error) {
	v.calls++
	v.gotToken = token
	if v.err != nil {
		return nil, v.err
	}
	return v.ticket, nil
}

// stubUsers answers GetUserByClaim and nothing else. The embedded interface is
// nil, so any other call panics loudly rather than returning a zero value that a
// test might mistake for a real answer.
type stubUsers struct {
	userpb.UserAPIClient

	user   *userpb.User
	status rpc.Code
	err    error

	gotClaim string
	gotValue string
}

func (g *stubUsers) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	g.gotClaim, g.gotValue = req.Claim, req.Value
	if g.err != nil {
		return nil, g.err
	}
	code := g.status
	if code == rpc.Code_CODE_INVALID {
		code = rpc.Code_CODE_OK
	}
	return &userpb.GetUserByClaimResponse{
		Status: &rpc.Status{Code: code},
		User:   g.user,
	}, nil
}

// stubClients installs the stub behind service.UserProvider. The manager
// resolves the user provider through the service registry, so this is the only
// service it needs; a Gateway method here would never be called.
type stubClients struct {
	service.Clients
	users userpb.UserAPIClient
}

func (c *stubClients) UserProvider(context.Context) (userpb.UserAPIClient, error) {
	return c.users, nil
}

// currentUsers is what the installed resolver hands out. service.SetGlobal
// keeps the first resolver it is given, so the tests share one resolver and
// swap what it points at.
var currentUsers *stubUsers

func TestMain(m *testing.M) {
	service.SetGlobal(&stubClients{users: usersProxy{}})
	os.Exit(m.Run())
}

// usersProxy forwards to whichever stub the running test installed.
type usersProxy struct{ userpb.UserAPIClient }

func (usersProxy) GetUserByClaim(ctx context.Context, req *userpb.GetUserByClaimRequest, opts ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	return currentUsers.GetUserByClaim(ctx, req, opts...)
}

func useUsers(t *testing.T, g *stubUsers) {
	t.Helper()
	previous := currentUsers
	currentUsers = g
	t.Cleanup(func() { currentUsers = previous })
}

func testUser(username string) *userpb.User {
	return &userpb.User{
		Id:       &userpb.UserId{OpaqueId: "id-" + username, Idp: "cernbox"},
		Username: username,
		Mail:     username + "@cern.ch",
	}
}

func encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// ── tests ────────────────────────────────────────────────────────────────────

func TestAuthenticate(t *testing.T) {
	users := &stubUsers{user: testUser("gdelmont")}
	useUsers(t, users)

	validator := &stubValidator{ticket: &Ticket{Principal: "gdelmont", Realm: "CERN.CH"}}
	m := NewWithValidator(&config{}, validator)

	user, scopes, err := m.Authenticate(context.Background(), "", encode("spnego-token"))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.Username != "gdelmont" {
		t.Errorf("username = %q", user.Username)
	}
	if len(scopes) == 0 {
		t.Error("no scope was granted")
	}
	if users.gotClaim != "username" {
		t.Errorf("resolved by claim %q, want username", users.gotClaim)
	}
	if users.gotValue != "gdelmont" {
		t.Errorf("resolved value %q", users.gotValue)
	}
	// The manager must hand the validator the decoded bytes.
	if string(validator.gotToken) != "spnego-token" {
		t.Errorf("validator received %q, want the decoded token", validator.gotToken)
	}
}

// TestAuthenticateIgnoresTheClientID is the security property that matters
// here: the ticket is verified and the client id is not, so a client must not
// be able to name a different user alongside a valid ticket.
func TestAuthenticateIgnoresTheClientID(t *testing.T) {
	users := &stubUsers{user: testUser("gdelmont")}
	useUsers(t, users)

	m := NewWithValidator(&config{}, &stubValidator{
		ticket: &Ticket{Principal: "gdelmont", Realm: "CERN.CH"},
	})

	user, _, err := m.Authenticate(context.Background(), "root", encode("token"))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if users.gotValue != "gdelmont" {
		t.Errorf("resolved %q: the client id was used instead of the verified principal", users.gotValue)
	}
	if user.Username != "gdelmont" {
		t.Errorf("authenticated as %q, want the principal from the ticket", user.Username)
	}
}

func TestAuthenticateRejectsAnInvalidTicket(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})

	m := NewWithValidator(&config{}, &stubValidator{err: errors.New("signature did not verify")})

	_, _, err := m.Authenticate(context.Background(), "", encode("token"))
	if err == nil {
		t.Fatal("an invalid ticket should not authenticate")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("error is %T, want InvalidCredentials", err)
	}
	// The reason belongs in the log, not in the response: it is equally useful
	// to an administrator and to someone probing the endpoint.
	if strings.Contains(err.Error(), "signature did not verify") {
		t.Errorf("the validator's reason leaked to the client: %v", err)
	}
}

func TestAuthenticateRejectsMalformedInput(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})
	validator := &stubValidator{ticket: &Ticket{Principal: "gdelmont", Realm: "CERN.CH"}}
	m := NewWithValidator(&config{}, validator)

	for _, secret := range []string{"", "not base64!!", "!!!"} {
		if _, _, err := m.Authenticate(context.Background(), "", secret); err == nil {
			t.Errorf("secret %q should be rejected", secret)
		}
	}
	if validator.calls != 0 {
		t.Errorf("the validator was called %d times for input that never decoded", validator.calls)
	}
}

// TestAuthenticateChecksTheRealm: a keytab can hold cross-realm keys, so a
// deployment that serves one realm has to say so and have it enforced.
func TestAuthenticateChecksTheRealm(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})

	m := NewWithValidator(&config{Realm: "CERN.CH"}, &stubValidator{
		ticket: &Ticket{Principal: "gdelmont", Realm: "EXAMPLE.ORG"},
	})

	_, _, err := m.Authenticate(context.Background(), "", encode("token"))
	if err == nil {
		t.Fatal("a ticket from another realm should be rejected")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("error is %T, want InvalidCredentials", err)
	}
}

func TestAuthenticateRealmComparisonIsCaseInsensitive(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})

	m := NewWithValidator(&config{Realm: "cern.ch"}, &stubValidator{
		ticket: &Ticket{Principal: "gdelmont", Realm: "CERN.CH"},
	})

	if _, _, err := m.Authenticate(context.Background(), "", encode("token")); err != nil {
		t.Errorf("realm comparison should ignore case: %v", err)
	}
}

func TestAuthenticateAcceptsAnyRealmWhenUnset(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})

	m := NewWithValidator(&config{}, &stubValidator{
		ticket: &Ticket{Principal: "gdelmont", Realm: "EXAMPLE.ORG"},
	})

	if _, _, err := m.Authenticate(context.Background(), "", encode("token")); err != nil {
		t.Errorf("no configured realm should accept any: %v", err)
	}
}

func TestAuthenticateRejectsAnEmptyPrincipal(t *testing.T) {
	useUsers(t, &stubUsers{user: testUser("gdelmont")})

	m := NewWithValidator(&config{}, &stubValidator{ticket: &Ticket{Realm: "CERN.CH"}})

	if _, _, err := m.Authenticate(context.Background(), "", encode("token")); err == nil {
		t.Fatal("a ticket with no principal should be rejected")
	}
}

// TestAuthenticateUnknownPrincipal: a genuine ticket naming somebody with no
// account here is a real situation, and it is not a credentials problem.
func TestAuthenticateUnknownPrincipal(t *testing.T) {
	useUsers(t, &stubUsers{status: rpc.Code_CODE_NOT_FOUND})

	m := NewWithValidator(&config{}, &stubValidator{
		ticket: &Ticket{Principal: "stranger", Realm: "CERN.CH"},
	})

	_, _, err := m.Authenticate(context.Background(), "", encode("token"))
	if err == nil {
		t.Fatal("an unknown principal should not authenticate")
	}
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Errorf("error is %T, want NotFound", err)
	}
	if !strings.Contains(err.Error(), "stranger") {
		t.Errorf("the error should name the principal, got %v", err)
	}
}

func TestAuthenticateWithACustomUserClaim(t *testing.T) {
	users := &stubUsers{user: testUser("gdelmont")}
	useUsers(t, users)

	m := NewWithValidator(&config{UserClaim: "mail"}, &stubValidator{
		ticket: &Ticket{Principal: "gdelmont", Realm: "CERN.CH"},
	})

	if _, _, err := m.Authenticate(context.Background(), "", encode("token")); err != nil {
		t.Fatal(err)
	}
	if users.gotClaim != "mail" {
		t.Errorf("claim = %q, want the configured mail", users.gotClaim)
	}
}

func TestConfigDefaults(t *testing.T) {
	var c config
	c.ApplyDefaults()

	if c.UserClaim != "username" {
		t.Errorf("UserClaim = %q, want username", c.UserClaim)
	}
	if c.MaxClockSkew() != 300_000_000_000 {
		t.Errorf("MaxClockSkew = %v, want the Kerberos default of five minutes", c.MaxClockSkew())
	}
}

func TestNewRequiresAKeytab(t *testing.T) {
	_, err := New(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("a manager with no keytab should not be constructed")
	}
	if !strings.Contains(err.Error(), "keytab") {
		t.Errorf("the error should say what is missing, got %v", err)
	}
}

// TestNewRejectsAnUnreadableKeytab: failing at startup beats failing at the
// first login, when the cause is much harder to see.
func TestNewRejectsAnUnreadableKeytab(t *testing.T) {
	_, err := New(context.Background(), map[string]any{
		"keytab": "/nonexistent/service.keytab",
	})
	if err == nil {
		t.Fatal("a missing keytab should fail at construction")
	}
}

func TestNewRejectsAMalformedKeytab(t *testing.T) {
	path := t.TempDir() + "/broken.keytab"
	if err := os.WriteFile(path, []byte("this is not a keytab"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(context.Background(), map[string]any{"keytab": path}); err == nil {
		t.Fatal("a malformed keytab should fail at construction")
	}
}
