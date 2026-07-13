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

package admin

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// peerCred are a socket peer's OS credentials, captured from SO_PEERCRED.
type peerCred struct {
	uid uint32
	gid uint32
	pid int32
}

// peerCredAuthInfo carries the peer credentials as gRPC AuthInfo.
type peerCredAuthInfo struct {
	credentials.CommonAuthInfo
	cred peerCred
}

func (peerCredAuthInfo) AuthType() string { return "peercred" }

func peerCredFrom(ctx context.Context) (peerCred, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return peerCred{}, false
	}
	ai, ok := p.AuthInfo.(peerCredAuthInfo)
	if !ok {
		return peerCred{}, false
	}
	return ai.cred, true
}

// socketOff is the `socket` config value that disables local root.
const socketOff = "off"

// DefaultSocketPaths are the well-known local-root socket locations, in the
// order the server binds and the CLI probes them: the system runtime dir, then
// the per-user one (rootless).
func DefaultSocketPaths() []string {
	paths := []string{"/run/reva/admin.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "reva", "admin.sock"))
	}
	return paths
}

// startSocket serves the AdminAPI on a local-root Unix socket, authenticating
// callers by OS credentials (SO_PEERCRED) instead of a token. Binding a default
// path is best-effort; an explicit path fails closed.
func (s *svc) startSocket() error {
	if s.socket == socketOff {
		return nil
	}
	explicit := s.socket != ""
	if !localRootSupported() {
		if explicit {
			return fmt.Errorf("admin: socket %q configured, but local root is only supported on Linux", s.socket)
		}
		return nil
	}

	candidates := DefaultSocketPaths()
	if explicit {
		candidates = []string{s.socket}
	}
	for _, path := range candidates {
		// Best-effort: a system dir like /run/reva is the deployment's to
		// provide.
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.Remove(path) // a leftover socket from an unclean shutdown blocks the bind
		ln, err := net.Listen("unix", path)
		if err != nil {
			if explicit {
				return fmt.Errorf("admin: listening on socket %q: %w", path, err)
			}
			continue
		}
		s.socketPath = path
		if err := s.secureSocket(); err != nil {
			_ = ln.Close()
			if explicit {
				return err
			}
			continue
		}
		srv := grpc.NewServer(
			grpc.Creds(peerCredentials()),
			grpc.UnaryInterceptor(s.localRootUnary),
			grpc.StreamInterceptor(s.localRootStream),
		)
		adminpb.RegisterAdminAPIServer(srv, s)
		s.socketServer = srv
		go func() { _ = srv.Serve(ln) }()
		if s.logger != nil {
			s.logger.Info().Str("socket", path).Msg("admin: serving local root on unix socket")
		}
		return nil
	}
	if s.logger != nil {
		s.logger.Warn().Msg("admin: no local-root socket could be opened; local root is disabled")
	}
	return nil
}

// secureSocket sets the socket's mode to the local-root policy: 0660 owned by
// the configured group, or 0600 without one.
func (s *svc) secureSocket() error {
	if s.socketGroup == "" {
		return os.Chmod(s.socketPath, 0o600)
	}
	g, err := user.LookupGroup(s.socketGroup)
	if err != nil {
		return fmt.Errorf("admin: socket_group %q: %w", s.socketGroup, err)
	}
	gid, _ := strconv.Atoi(g.Gid)
	if err := os.Chown(s.socketPath, os.Getuid(), gid); err != nil {
		return fmt.Errorf("admin: chowning socket to group %q (reva must run as root or be a member of it): %w", s.socketGroup, err)
	}
	return os.Chmod(s.socketPath, 0o660)
}

func (s *svc) stopSocket() {
	if s.socketServer != nil {
		s.socketServer.GracefulStop()
		_ = os.Remove(s.socketPath)
	}
}

// localRootUnary authorizes a socket caller and runs the handler with the
// elevated context.
func (s *svc) localRootUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx, err := s.authorizeLocalRoot(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// localRootStream is the streaming counterpart of localRootUnary, so streaming
// admin RPCs work over the socket too.
func (s *svc) localRootStream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, err := s.authorizeLocalRoot(ss.Context(), info.FullMethod)
	if err != nil {
		return err
	}
	return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
}

// authorizeLocalRoot authenticates a socket caller by its OS credentials and,
// if permitted, auto-elevates it: it mints a short-TTL admin token and injects
// it into the context and outgoing metadata, so the request runs exactly like
// a remotely-elevated one, fan-out included.
func (s *svc) authorizeLocalRoot(ctx context.Context, method string) (context.Context, error) {
	// Socket requests bypass the interceptor chain, so carry the logger.
	if s.logger != nil {
		ctx = appctx.WithLogger(ctx, s.logger)
	}
	cred, ok := peerCredFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "admin: no peer credentials on the control socket")
	}
	who := osUserName(cred.uid)
	if !s.socketAllowed(cred) {
		admin.Audit(ctx, admin.AuditEvent{Action: "local_root", Actor: who, Granted: false,
			Err: fmt.Errorf("uid %d is not in group %q", cred.uid, s.socketGroup)})
		return nil, status.Errorf(codes.PermissionDenied, "admin: uid %d is not permitted local root", cred.uid)
	}

	u := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: fmt.Sprintf("uid:%d", cred.uid), Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username: who,
		Groups:   []string{s.adminGroup},
	}
	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: building admin scope: %v", err)
	}
	tkn, err := s.tokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: minting admin token: %v", err)
	}
	ctx = appctx.ContextSetUser(ctx, u)
	ctx = appctx.ContextSetScopes(ctx, scopes)
	ctx = appctx.ContextSetToken(ctx, tkn)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn)

	admin.Audit(ctx, admin.AuditEvent{Action: "local_root", Actor: who, Granted: true,
		Fields: map[string]string{"method": method, "uid": strconv.Itoa(int(cred.uid))}})
	return ctx, nil
}

// wrappedServerStream overrides a server stream's context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context { return w.ctx }

// socketAllowed reports whether a peer may take local root: with no group
// configured the socket's file permissions are the sole gate; with one, the
// peer's uid must belong to it (root always may).
func (s *svc) socketAllowed(cred peerCred) bool {
	if s.socketGroup == "" {
		return true
	}
	if cred.uid == 0 {
		return true
	}
	return uidInGroup(cred.uid, s.socketGroup)
}

func osUserName(uid uint32) string {
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		return u.Username
	}
	return fmt.Sprintf("uid:%d", uid)
}

// uidInGroup reports whether the uid belongs to the group (primary or
// supplementary).
func uidInGroup(uid uint32, group string) bool {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return false
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	return slices.Contains(gids, g.Gid)
}
