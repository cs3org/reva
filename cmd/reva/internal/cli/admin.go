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

package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// adminMaxRecvMsgSize lifts the 4 MiB default, so that a fleet-wide unary
// fan-out (the results of many instances in one response) fits.
const adminMaxRecvMsgSize = 64 << 20

// expiryMargin keeps a token that expires during the next call out of use.
const expiryMargin = 30 * time.Second

// AdminClient is a connection to the Admin API. Conn also serves the typed
// admin services (SharesAdmin, SpacesAdmin, StorageAdmin).
type AdminClient struct {
	adminpb.AdminAPIClient
	Conn *grpc.ClientConn
	// Socket is true for the local admin socket, which needs no token.
	Socket bool
}

// socketPaths are the well-known locations of the admin socket. They are
// never rebranded. Tests replace them.
var socketPaths = func() []string {
	paths := []string{"/run/reva/admin.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "reva", "admin.sock"))
	}
	return paths
}

// Admin returns an Admin API client and ctx for an admin-plane call. It
// uses the local socket if one answers, which needs no token. Else it uses
// the admin host with an admin token: the one in memory, the one that
// `reva elevate` stored for the current login, or a new one from an
// automatic step-up. The token of the step-up stays in memory only. An admin
// host from --admin-host or REVA_ADMIN_HOST skips the socket.
func (r *Runtime) Admin(ctx context.Context) (*AdminClient, context.Context, error) {
	r.usedAdmin = true
	c, err := r.adminClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	ctx = r.adminHeaders(ctx)
	if c.Socket {
		// The socket knows the caller from the peer credentials, so a user
		// token of ctx must not go with the call.
		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			md = md.Copy()
			md.Delete(appctx.TokenHeader)
			ctx = metadata.NewOutgoingContext(ctx, md)
		}
		return c, ctx, nil
	}
	t, err := r.adminToken(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	return c, WithToken(ctx, t), nil
}

// AdminContext returns ctx for an admin-plane call, for the callers that
// build their own typed client on AdminClient.Conn.
func (r *Runtime) AdminContext(ctx context.Context) (context.Context, error) {
	_, ctx, err := r.Admin(ctx)
	return ctx, err
}

// adminHeaders adds the reason and the SUDO_USER to the outgoing metadata.
// Both are only for the audit: the server does not trust them.
func (r *Runtime) adminHeaders(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	if r.Reason != "" {
		md.Set(admin.ReasonHeader, r.Reason)
	}
	if u := os.Getenv("SUDO_USER"); u != "" {
		md.Set(admin.SudoUserHeader, u)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func (r *Runtime) adminClient(ctx context.Context) (*AdminClient, error) {
	if r.admin != nil {
		return r.admin, nil
	}
	if !r.adminHostExplicit {
		for _, p := range socketPaths() {
			if fi, err := os.Stat(p); err != nil || fi.Mode()&os.ModeSocket == 0 {
				continue
			}
			c, err := r.dialAdmin("unix://" + p)
			if err != nil {
				continue
			}
			if socketWorks(ctx, c) {
				r.admin = c
				return c, nil
			}
			_ = c.Conn.Close()
		}
	}
	c, err := r.dialAdminHost()
	if err != nil {
		return nil, err
	}
	r.admin = c
	return c, nil
}

func (r *Runtime) dialAdminHost() (*AdminClient, error) {
	host := r.AdminHost
	if host == "" {
		// The Admin API is served on the gateway listener unless the
		// deployment puts it elsewhere.
		host = r.GatewayAddr
	}
	if host == "" {
		return nil, errors.New("admin host not set: pass --admin-host, set REVA_ADMIN_HOST or run `reva config set admin-host <host:port>`")
	}
	return r.dialAdmin(host)
}

func (r *Runtime) dialAdmin(host string) (*AdminClient, error) {
	socket := strings.HasPrefix(host, "unix:")
	creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: r.SkipVerify})
	if r.Insecure || socket {
		creds = insecure.NewCredentials()
	}
	opts := append(r.DialOptions(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(adminMaxRecvMsgSize)),
		grpc.WithTransportCredentials(creds))
	conn, err := grpc.NewClient(host, opts...)
	if err != nil {
		return nil, err
	}
	return &AdminClient{AdminAPIClient: adminpb.NewAdminAPIClient(conn), Conn: conn, Socket: socket}, nil
}

// socketWorks probes the socket with a cheap call. A socket that denies the
// caller, or that nobody serves, returns false, so that the caller falls back.
func socketWorks(ctx context.Context, c *AdminClient) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := c.GetServerInfo(ctx, &adminpb.GetServerInfoRequest{})
	return err == nil
}

func (r *Runtime) adminToken(ctx context.Context, c *AdminClient) (string, error) {
	if r.adminTok != "" && usable(r.adminTok) {
		return r.adminTok, nil
	}
	login, err := r.LoginToken(ctx)
	if err != nil {
		return "", err
	}
	if t := storedAdminToken(login); t != "" {
		r.adminTok = t
		return t, nil
	}
	need := r.need
	if need == "" {
		need = "this command"
	}
	if r.NoElevate {
		return "", fmt.Errorf("%s needs admin rights: run `reva elevate` first, or do not pass --no-elevate", need)
	}
	res, err := c.RequestAdmin(WithToken(ctx, login), &adminpb.RequestAdminRequest{})
	if err != nil {
		return "", fmt.Errorf("%s needs admin rights: %w", need, err)
	}
	r.adminTok = res.Token
	return res.Token, nil
}

// Elevate steps up with the login token and stores the admin token for this
// login. It is the only place that writes an admin token to disk.
func (r *Runtime) Elevate(ctx context.Context) (time.Time, error) {
	r.usedAdmin = true
	login, err := r.LoginToken(ctx)
	if err != nil {
		return time.Time{}, err
	}
	// The socket needs no token, so elevate always uses the admin host.
	c, err := r.dialAdminHost()
	if err != nil {
		return time.Time{}, err
	}
	defer c.Conn.Close()
	if c.Socket {
		return time.Time{}, errors.New("the local admin socket needs no elevation")
	}
	res, err := c.RequestAdmin(WithToken(r.adminHeaders(ctx), login), &adminpb.RequestAdminRequest{})
	if err != nil {
		return time.Time{}, err
	}
	b, err := json.Marshal(storedAdmin{Login: loginKey(login), Token: res.Token})
	if err != nil {
		return time.Time{}, err
	}
	if err := writeState("admin-token", b); err != nil {
		return time.Time{}, err
	}
	r.adminTok = res.Token
	return TokenExpiry(res.Token), nil
}

// storedAdmin is the admin-token file. Login is a hash of the login token,
// so that the admin token is used only while that login is current.
type storedAdmin struct {
	Login string `json:"login"`
	Token string `json:"token"`
}

func loginKey(login string) string {
	h := sha256.Sum256([]byte(login))
	return hex.EncodeToString(h[:])
}

func storedAdminToken(login string) string {
	b, err := readState("admin-token", ".reva-admin-token")
	if err != nil {
		return ""
	}
	var s storedAdmin
	if json.Unmarshal(b, &s) != nil || s.Login != loginKey(login) || !usable(s.Token) {
		return ""
	}
	return s.Token
}

// TokenExpiry reads the exp claim of a reva token, without a check of the
// signature: only the server can check it, and the expires_at of a response
// can differ when the token manager overrides it. Zero means unknown.
func TokenExpiry(t string) time.Time {
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(t, &claims); err != nil || claims.ExpiresAt == nil {
		return time.Time{}
	}
	return claims.ExpiresAt.Time
}

// Left returns the time until exp in minutes, for a person.
func Left(exp time.Time) string {
	left := time.Until(exp).Round(time.Minute)
	if left < time.Minute {
		return "less than 1m"
	}
	return strings.TrimSuffix(left.String(), "0s")
}

func usable(t string) bool {
	exp := TokenExpiry(t)
	return exp.IsZero() || time.Until(exp) > expiryMargin
}

// AdminState describes the admin rights of the caller for whoami, without a
// step-up: "socket", "yes, 12m left" or "no".
func (r *Runtime) AdminState(ctx context.Context) string {
	if c, err := r.adminClient(ctx); err == nil && c.Socket {
		return "socket"
	}
	t := r.adminTok
	if t == "" {
		if login, err := r.LoginToken(ctx); err == nil {
			t = storedAdminToken(login)
		}
	}
	if t == "" || !usable(t) {
		return "no"
	}
	exp := TokenExpiry(t)
	if exp.IsZero() {
		return "yes"
	}
	return "yes, " + Left(exp) + " left"
}

// CanAdmin reports whether the caller can be an admin, without a step-up:
// CheckAdmin with the login token. It is false under --as, where the caller
// has chosen an identity.
func (r *Runtime) CanAdmin(ctx context.Context) bool {
	if r.As != "" {
		return false
	}
	c, err := r.adminClient(ctx)
	if err != nil {
		r.Debugf("admin: %v", err)
		return false
	}
	if c.Socket || (r.adminTok != "" && usable(r.adminTok)) {
		return true
	}
	login, err := r.LoginToken(ctx)
	if err != nil {
		return false
	}
	res, err := c.CheckAdmin(WithToken(ctx, login), &adminpb.CheckAdminRequest{})
	if err != nil {
		r.Debugf("admin: CheckAdmin: %v", err)
		return false
	}
	return res.Admin
}

// impersonate returns a user token for --as. Over gRPC it calls Impersonate
// with admin rights. When the Admin API cannot be reached and an HTTPS
// endpoint is known, it uses POST /admin/impersonate of that endpoint.
func (r *Runtime) impersonate(ctx context.Context) (string, error) {
	c, actx, err := r.Admin(ctx)
	if err == nil {
		var res *adminpb.ImpersonateResponse
		if res, err = c.Impersonate(actx, &adminpb.ImpersonateRequest{User: r.As, Reason: r.Reason}); err == nil {
			return res.Token, nil
		}
		err = fmt.Errorf("--as %s: %w", r.As, err)
	}
	if r.HTTPEndpoint == "" || !unreachable(err) {
		return "", err
	}
	login, lerr := r.LoginToken(ctx)
	if lerr != nil {
		return "", lerr
	}
	return ImpersonateHTTP(ctx, r.HTTPClient().GetNativeHTTP(), r.HTTPEndpoint, login, r.As, r.Reason)
}

func unreachable(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.Unavailable
}

// ImpersonateHTTP asks the admin HTTP service of endpoint for a user token
// of user. The admin token never reaches the client on this path.
func ImpersonateHTTP(ctx context.Context, client *http.Client, endpoint, login, user, reason string) (string, error) {
	body, err := json.Marshal(map[string]string{"user": user, "reason": reason})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(endpoint, "/")+"/admin/impersonate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-access-token", login)
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Token   string `json:"token"`
		Message string `json:"message"`
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return "", err
	}
	isJSON := json.Unmarshal(b, &out) == nil
	switch {
	case res.StatusCode == http.StatusOK && isJSON && out.Token != "":
		return out.Token, nil
	case isJSON && out.Message != "":
		return "", fmt.Errorf("--as %s: %s", user, out.Message)
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusMethodNotAllowed:
		return "", fmt.Errorf("--as %s: %s does not expose /admin/impersonate, and the Admin API cannot be reached", user, endpoint)
	}
	return "", fmt.Errorf("--as %s: %s answered %s", user, endpoint, res.Status)
}

// Confirm asks the person at the terminal to confirm prompt. --yes skips the
// question. Without a terminal and without --yes, it refuses, so that a
// script never changes data by accident.
func (r *Runtime) Confirm(prompt string) error {
	if r.Yes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("%s: refused without a terminal, pass --yes to confirm", prompt)
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if s := strings.ToLower(strings.TrimSpace(line)); s != "y" && s != "yes" {
		return errors.New("aborted")
	}
	return nil
}

// Warnf writes a line to stderr that -q does not hide, for the use of admin
// rights that the person did not ask for by name.
func (r *Runtime) Warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "reva: "+format+"\n", args...)
}

// ConfirmMutation asks for a confirmation before a command that changes data
// runs as another user with --as.
func (r *Runtime) ConfirmMutation(path string) error {
	if r.As == "" {
		return nil
	}
	return r.Confirm(fmt.Sprintf("run `%s` as %s", path, r.As))
}

// WarnUnusedReason warns when --reason was given, but no call used the
// admin plane, so the reason is in no audit line.
func (r *Runtime) WarnUnusedReason() {
	if r.Reason != "" && !r.usedAdmin {
		r.Warnf("--reason ignored: the command made no admin call")
	}
}
