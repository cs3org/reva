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
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// logBuffer collects the log lines of the servers, which write from their own
// goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// audits returns the audit lines with the given action.
func (b *logBuffer) audits(t *testing.T, action string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(b.buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if m["audit"] == true && m["action"] == action {
			out = append(out, m)
		}
	}
	return out
}

// auditingInvokable audits from inside its handler, like an invocation that
// changes state.
type auditingInvokable struct{}

func (auditingInvokable) Invocations() []invoke.InvocationSpec {
	return []invoke.InvocationSpec{{Name: "change", Kind: invoke.KindMutating}}
}

func (auditingInvokable) Invoke(ctx context.Context, _ string, _ map[string]any) (invoke.Result, error) {
	admin.Audit(ctx, admin.AuditEvent{Action: "handler_change"})
	return invoke.Result{}, nil
}

func withLogger(log *zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		return h(appctx.WithLogger(ctx, log), req)
	}
}

func serveGRPC(t *testing.T, network, addr string, srv *grpc.Server) string {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	return ln.Addr().String()
}

// startReasonFleet starts a control channel with one auditing instance of the
// service t.Name(), and returns the admin service that reaches it.
func startReasonFleet(t *testing.T, log *zerolog.Logger) *svc {
	t.Helper()
	ctrl := grpc.NewServer(grpc.UnaryInterceptor(withLogger(log)))
	controlpb.RegisterControlServer(ctrl, control.NewServer())
	ctrlAddr := serveGRPC(t, "tcp", "127.0.0.1:0", ctrl)

	nodeID := ctrlAddr + "/" + t.Name()
	invoke.RegisterInstance(nodeID, t.Name(), nil, auditingInvokable{}, nil)

	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	// The first registry set in the process stays, so add to that one.
	service.SetGlobalRegistry(reg)
	if err := service.GlobalRegistry().Add(registry.NewService(t.Name(), []registry.Node{
		registry.NewNode(nodeID, ctrlAddr, map[string]string{
			registry.MetaState:   registry.StateReady,
			registry.MetaControl: ctrlAddr,
		}),
	})); err != nil {
		t.Fatal(err)
	}

	s, _ := testService(t)
	s.logger = log
	return s
}

func invokeWithReason(t *testing.T, target, reason string, extra ...string) {
	t.Helper()
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := metadata.AppendToOutgoingContext(context.Background(), append([]string{admin.ReasonHeader, reason}, extra...)...)
	res, err := adminpb.NewAdminAPIClient(conn).Invoke(ctx, &adminpb.InvokeRequest{Service: t.Name(), Invocation: "change"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].Error != "" {
		t.Fatalf("Invoke results: %v", res.Results)
	}
}

func assertReason(t *testing.T, logs *logBuffer, action, reason string) {
	t.Helper()
	lines := logs.audits(t, action)
	if len(lines) != 1 {
		t.Fatalf("%d %q audit lines, want 1", len(lines), action)
	}
	if lines[0]["reason"] != reason {
		t.Errorf("%q audit reason = %v, want %q", action, lines[0]["reason"], reason)
	}
}

// TestInvokeReasonOverListener checks that the reason header of an Invoke is
// on the audit line of the admin service and on the audit line that the
// invocation handler writes in the owning service.
func TestInvokeReasonOverListener(t *testing.T) {
	logs := &logBuffer{}
	log := zerolog.New(logs)
	s := startReasonFleet(t, &log)

	srv := grpc.NewServer(grpc.UnaryInterceptor(withLogger(&log)))
	adminpb.RegisterAdminAPIServer(srv, s)
	addr := serveGRPC(t, "tcp", "127.0.0.1:0", srv)

	invokeWithReason(t, addr, "INC0042 fix quota")
	assertReason(t, logs, "invoke", "INC0042 fix quota")
	assertReason(t, logs, "handler_change", "INC0042 fix quota")
}

// TestInvokeReasonOverSocket checks the same over the local socket, which has
// its own interceptors, and that its audit names the peer pid and the sudo user.
func TestInvokeReasonOverSocket(t *testing.T) {
	if !localRootSupported() {
		t.Skip("local root needs SO_PEERCRED")
	}
	logs := &logBuffer{}
	log := zerolog.New(logs)
	s := startReasonFleet(t, &log)

	// A unix socket path has a short length limit, so do not use t.TempDir.
	dir, err := os.MkdirTemp("", "reva")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	s.socket = filepath.Join(dir, "admin.sock")
	if err := s.startSocket(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stopSocket)

	invokeWithReason(t, "unix://"+s.socket, "INC0043", admin.SudoUserHeader, "alice")
	assertReason(t, logs, "invoke", "INC0043")
	assertReason(t, logs, "handler_change", "INC0043")

	lines := logs.audits(t, "local_root")
	if len(lines) != 1 {
		t.Fatalf("%d local_root audit lines, want 1", len(lines))
	}
	if lines[0]["pid"] == "" || lines[0]["pid"] == nil || lines[0]["sudo_user"] != "alice" || lines[0]["reason"] != "INC0043" {
		t.Errorf("local_root audit = %v, want pid, sudo_user alice and the reason", lines[0])
	}
}

type fakeMachineAuth struct{}

func (fakeMachineAuth) Authenticate(_ context.Context, user, _ string) (*userpb.User, map[string]*authpb.Scope, error) {
	scopes, err := scope.AddOwnerScope(nil)
	return &userpb.User{Id: &userpb.UserId{OpaqueId: user}, Username: user}, scopes, err
}

// TestImpersonateAuditReason checks that the request reason of Impersonate
// still works, and that the header is logged when both are set.
func TestImpersonateAuditReason(t *testing.T) {
	for _, c := range []struct {
		name, header, field string
		want                map[string]any
	}{
		{"field only", "", "INC1", map[string]any{"reason": "INC1"}},
		{"header only", "INC2", "", map[string]any{"reason": "INC2"}},
		{"same", "INC3", "INC3", map[string]any{"reason": "INC3"}},
		{"different", "INC4", "INC5", map[string]any{"reason": "INC4", "request_reason": "INC5"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			logs := &logBuffer{}
			log := zerolog.New(logs)
			s, tm := testService(t)
			s.machineAuth = fakeMachineAuth{}
			s.userTokenManager = tm

			ctx := appctx.WithLogger(context.Background(), &log)
			if c.header != "" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(admin.ReasonHeader, c.header))
			}
			if _, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "bob", Reason: c.field}); err != nil {
				t.Fatal(err)
			}
			lines := logs.audits(t, "impersonate")
			if len(lines) != 1 {
				t.Fatalf("%d impersonate audit lines, want 1", len(lines))
			}
			for _, k := range []string{"reason", "request_reason"} {
				if lines[0][k] != c.want[k] {
					t.Errorf("%s = %v, want %v", k, lines[0][k], c.want[k])
				}
			}
		})
	}
}
