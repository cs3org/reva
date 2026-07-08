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

// Package admin implements the reva Admin API gRPC service (reva.admin.v1beta1),
// an authenticated operator surface for fleet introspection and bounded,
// audited actions. It binds its own admin-only server block and requires the
// admin scope on every method except RequestAdmin — the step-up door.
package admin

import (
	"context"
	"maps"
	"slices"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/manager/machine"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/token"
	tokenregistry "github.com/cs3org/reva/v3/pkg/token/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultAdminTTL = "15m"

func init() {
	rgrpc.Register("admin", New)
}

type config struct {
	// AdminGroup is the directory group whose members may step up. Unset
	// disables the Admin API entirely.
	AdminGroup string `mapstructure:"admin_group"`
	// AdminTTL is the lifetime of a minted admin token (default 15m).
	AdminTTL string `mapstructure:"admin_ttl"`

	TokenManager  string                    `mapstructure:"token_manager"`
	TokenManagers map[string]map[string]any `mapstructure:"token_managers"`

	// MachineAuthAPIKey enables Impersonate: the api-key the machine auth
	// manager uses to mint a user-scoped token for a target user. Unset
	// disables impersonation.
	MachineAuthAPIKey string `mapstructure:"machine_auth_apikey"`

	// Socket is the Unix socket path serving local root: callers authenticated
	// by OS credentials (SO_PEERCRED) instead of a token. Empty uses the
	// well-known default path; "off" disables it.
	Socket string `mapstructure:"socket"`
	// SocketGroup restricts local root to members of this Unix group; unset
	// leaves the socket's file permissions as the only gate.
	SocketGroup string `mapstructure:"socket_group"`
}

func (c *config) ApplyDefaults() {
	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}
	if c.AdminTTL == "" {
		c.AdminTTL = defaultAdminTTL
	}
}

type svc struct {
	adminpb.UnimplementedAdminAPIServer

	conf          *config
	adminGroup    string
	adminTTL      time.Duration
	tokenManager  token.Manager
	machineAuth   auth.Manager
	machineAPIKey string
	startTime     time.Time

	// socket is the configured value; socketPath the path actually bound.
	socket       string
	socketPath   string
	socketGroup  string
	socketServer *grpc.Server
	// logger is carried onto socket requests, which bypass the interceptor
	// chain that would normally provide one.
	logger *zerolog.Logger
}

// New returns the Admin API service. It refuses to start without admin_group
// (fail closed).
func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	if c.AdminGroup == "" {
		return nil, errors.New("admin: admin_group is required; the Admin API is disabled without it")
	}
	ttl, err := time.ParseDuration(c.AdminTTL)
	if err != nil {
		return nil, errors.Wrapf(err, "admin: invalid admin_ttl %q", c.AdminTTL)
	}

	tm, err := newTokenManager(c, ttl)
	if err != nil {
		return nil, err
	}

	// Optionally wire the machine auth manager for impersonation.
	var machineAuth auth.Manager
	if c.MachineAuthAPIKey != "" {
		machineAuth, err = machine.New(ctx, map[string]any{"api_key": c.MachineAuthAPIKey})
		if err != nil {
			return nil, errors.Wrap(err, "admin: error creating machine auth manager")
		}
	}

	s := &svc{
		conf:          &c,
		adminGroup:    c.AdminGroup,
		adminTTL:      ttl,
		tokenManager:  tm,
		machineAuth:   machineAuth,
		machineAPIKey: c.MachineAuthAPIKey,
		startTime:     time.Now(),
		socket:        c.Socket,
		socketGroup:   c.SocketGroup,
		logger:        appctx.GetLogger(ctx),
	}
	// Fail closed: a configured socket that cannot be opened is an error.
	if err := s.startSocket(); err != nil {
		return nil, err
	}
	return s, nil
}

// newTokenManager builds the manager minting short-TTL admin tokens. The
// signing secret defaults to the shared JWT secret so the auth interceptor can
// verify what we mint.
func newTokenManager(c config, ttl time.Duration) (token.Manager, error) {
	h, ok := tokenregistry.NewFuncs[c.TokenManager]
	if !ok {
		return nil, errtypes.NotFound("admin: token manager does not exist: " + c.TokenManager)
	}
	tmConf := map[string]any{}
	maps.Copy(tmConf, c.TokenManagers[c.TokenManager])
	if _, ok := tmConf["expires"]; !ok {
		tmConf["expires"] = int64(ttl.Seconds())
	}
	tm, err := h(tmConf)
	if err != nil {
		return nil, errors.Wrap(err, "admin: error creating token manager")
	}
	return tm, nil
}

func (s *svc) Close() error {
	s.stopSocket()
	return nil
}

// UnprotectedEndpoints returns none: every method needs a valid token.
// RequestAdmin is satisfied by the user scope, everything else by the admin
// scope (enforced by the auth interceptor).
func (s *svc) UnprotectedEndpoints() []string {
	return nil
}

func (s *svc) Register(ss *grpc.Server) {
	adminpb.RegisterAdminAPIServer(ss, s)
	// Invocations are not served here; they live on the per-process control
	// channel.
}

// RequestAdmin is the step-up door: it checks the (already authenticated)
// caller's group membership and, on success, mints a short-TTL admin-only
// token. Grant and deny are both audited.
func (s *svc) RequestAdmin(ctx context.Context, _ *adminpb.RequestAdminRequest) (*adminpb.RequestAdminResponse, error) {
	u, err := s.caller(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "admin: cannot identify caller: %v", err)
	}

	if !slices.Contains(u.Groups, s.adminGroup) {
		denied := errtypes.PermissionDenied("user is not a member of " + s.adminGroup)
		admin.Audit(ctx, admin.AuditEvent{Action: "request_admin", Actor: u.Username, Granted: false, Err: denied})
		return nil, status.Errorf(codes.PermissionDenied, "admin: %s", denied.Error())
	}

	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: building admin scope: %v", err)
	}
	tkn, err := s.tokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: minting admin token: %v", err)
	}
	expires := time.Now().Add(s.adminTTL)

	admin.Audit(ctx, admin.AuditEvent{Action: "request_admin", Actor: u.Username, Granted: true})
	return &adminpb.RequestAdminResponse{
		Token:     tkn,
		ExpiresAt: expires.Unix(),
	}, nil
}

// caller returns the authenticated user, falling back to dismantling the token
// directly, and resolves groups through the gateway if the user carries none.
func (s *svc) caller(ctx context.Context) (*userpb.User, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u == nil {
		tkn, ok := appctx.ContextGetToken(ctx)
		if !ok || tkn == "" {
			return nil, errtypes.NotFound("no access token in request")
		}
		var err error
		if u, _, err = s.tokenManager.DismantleToken(ctx, tkn); err != nil {
			return nil, err
		}
	}
	if len(u.Groups) == 0 {
		if gw, err := service.Gateway(ctx); err == nil {
			if res, err := gw.GetUserGroups(ctx, &userpb.GetUserGroupsRequest{UserId: u.Id}); err == nil &&
				res.Status != nil && res.Status.Code == rpcv1beta1.Code_CODE_OK {
				u.Groups = res.Groups
			}
		}
	}
	return u, nil
}
