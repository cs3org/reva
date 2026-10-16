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

// Package kerberos authenticates a user from a Kerberos ticket, so that a
// client holding a TGT can reach reva directly without an intermediate identity
// provider.
//
// The client presents a SPNEGO token, which this manager validates against the
// service keytab. What it gets back is a client principal, which it resolves to
// a user through the user provider. The principal is authoritative: the client id
// supplied alongside the token is ignored, because anything the client says
// about who it is has not been verified and the ticket has.
//
// # Mutual authentication
//
// Mutual authentication is not supported, by construction. This manager runs in
// the auth provider, which is a different process from the HTTP frontend that
// received the request, and the CS3 Authenticate RPC returns a user and a token
// with nowhere to put a SPNEGO response token. A client must therefore not set
// GSS_C_MUTUAL_FLAG. This is the usual arrangement for SPNEGO over HTTPS, where
// the TLS certificate already authenticates the server.
//
// # Replay
//
// The replay cache is per-process and in memory, so several auth provider
// replicas do not share one. Under TLS an attacker cannot observe an AP-REQ to
// replay it, which is what makes this acceptable; a deployment that wants
// replay protection across replicas needs a shared cache.
package kerberos

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("kerberos", New)
}

type config struct {
	// Keytab is the path to the service keytab holding the key for
	// ServicePrincipal.
	Keytab string `docs:";Path to the service keytab." mapstructure:"keytab"`
	// ServicePrincipal is the SPN clients request a ticket for, such as
	// HTTP/cernbox.cern.ch. Empty accepts any principal the keytab holds.
	ServicePrincipal string `docs:";The service principal name clients authenticate to. Empty accepts any principal in the keytab." mapstructure:"service_principal"`
	// Realm is the Kerberos realm to accept. Empty accepts any realm the
	// keytab can validate.
	Realm string `docs:";The Kerberos realm to accept. Empty accepts any." mapstructure:"realm"`
	// UserClaim is the claim the username from the ticket is resolved
	// against.
	UserClaim string `docs:"username;The user claim the username from the ticket is resolved against." mapstructure:"user_claim"`
	// MaxClockSkewSeconds bounds the difference between the client's clock and
	// this host's. Kerberos is sensitive to it, and the default matches the
	// protocol's own.
	MaxClockSkewSeconds int `docs:"300;Maximum tolerated clock skew, in seconds." mapstructure:"max_clock_skew_seconds"`
}

func (c *config) ApplyDefaults() {
	if c.UserClaim == "" {
		c.UserClaim = "username"
	}
	if c.MaxClockSkewSeconds == 0 {
		c.MaxClockSkewSeconds = 300
	}
}

// Ticket is what a validated SPNEGO token yields.
type Ticket struct {
	// Username is the first component of the client principal, without realm
	// or instance: "gdelmont" for gdelmont/admin@CERN.CH.
	Username string
	// Realm is the realm the client authenticated in.
	Realm string
}

// Validator turns a SPNEGO token into the identity it proves.
//
// It is an interface so the manager's own behaviour — configuration, realm
// checks, user resolution, error mapping — can be tested without a KDC.
// Producing a genuine AP-REQ needs a live realm, which belongs in an
// integration test, not in a unit one.
type Validator interface {
	// Validate verifies the token against the service keytab and returns the
	// client identity it proves.
	Validate(ctx context.Context, token []byte) (*Ticket, error)
}

type manager struct {
	conf      *config
	validator Validator
}

// New returns an auth manager that authenticates Kerberos tickets.
func New(ctx context.Context, m map[string]any) (auth.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, errors.Wrap(err, "kerberos: error decoding config")
	}
	if c.Keytab == "" {
		return nil, errors.New("kerberos: keytab is required")
	}

	v, err := newKeytabValidator(&c)
	if err != nil {
		return nil, err
	}
	return &manager{conf: &c, validator: v}, nil
}

// NewWithValidator returns a manager using the given validator. Tests use it to
// stand in for a KDC.
func NewWithValidator(c *config, v Validator) auth.Manager {
	c.ApplyDefaults()
	return &manager{conf: c, validator: v}
}

// Authenticate validates a SPNEGO token and resolves the user it names.
//
// clientID is ignored on purpose. The ticket says who the client is and has
// been verified; the client id has not.
func (m *manager) Authenticate(ctx context.Context, clientID, clientSecret string) (*userpb.User, map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx)

	token, err := base64.StdEncoding.DecodeString(strings.TrimSpace(clientSecret))
	if err != nil {
		return nil, nil, errtypes.InvalidCredentials("kerberos: token is not valid base64")
	}
	if len(token) == 0 {
		return nil, nil, errtypes.InvalidCredentials("kerberos: empty token")
	}

	ticket, err := m.validator.Validate(ctx, token)
	if err != nil {
		log.Debug().Err(err).Msg("kerberos: token validation failed")
		// The reason a ticket failed is useful to an administrator reading the
		// log and useful to an attacker probing the endpoint, so it stays in
		// the log and the client is told only that it failed.
		return nil, nil, errtypes.InvalidCredentials("kerberos: could not validate the ticket")
	}

	if m.conf.Realm != "" && !strings.EqualFold(ticket.Realm, m.conf.Realm) {
		log.Debug().Str("realm", ticket.Realm).Str("expected", m.conf.Realm).
			Msg("kerberos: ticket from an unexpected realm")
		return nil, nil, errtypes.InvalidCredentials("kerberos: ticket is from another realm")
	}
	if ticket.Username == "" {
		return nil, nil, errtypes.InvalidCredentials("kerberos: ticket names no user")
	}

	user, err := m.resolveUser(ctx, ticket.Username)
	if err != nil {
		return nil, nil, err
	}

	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "kerberos: error creating scope")
	}

	log.Debug().Str("username", ticket.Username).Str("realm", ticket.Realm).
		Str("user", user.Username).Msg("kerberos: authenticated")
	return user, scopes, nil
}

// resolveUser maps the username from a ticket to a reva user.
//
// It asks the user provider directly, discovered through the service registry,
// rather than routing through the gateway at an address from the shared
// configuration. Resolving a username to a user is the user provider's own job,
// and going straight there means one fewer hop on the authentication path and
// one fewer address to keep correct in a config file.
func (m *manager) resolveUser(ctx context.Context, username string) (*userpb.User, error) {
	users, err := service.UserProvider(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "kerberos: error getting the user provider")
	}

	res, err := users.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: m.conf.UserClaim,
		Value: username,
	})
	switch {
	case err != nil:
		return nil, errors.Wrap(err, "kerberos: error resolving the user")
	case res.Status.Code == rpc.Code_CODE_NOT_FOUND:
		// The ticket is genuine but names somebody this deployment does not
		// know. That is a real situation — a valid principal in the realm with
		// no account here — and saying so plainly beats a generic failure.
		return nil, errtypes.NotFound(fmt.Sprintf("kerberos: no user with %s %q", m.conf.UserClaim, username))
	case res.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(res.Status.Message)
	}
	return res.GetUser(), nil
}

// MaxClockSkew returns the configured tolerance.
func (c *config) MaxClockSkew() time.Duration {
	return time.Duration(c.MaxClockSkewSeconds) * time.Second
}
