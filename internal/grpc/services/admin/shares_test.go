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
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// fakeProvider stands for a share provider, a link provider or the spaces
// registry: it answers each invocation with its handler, or with its list
// for a stream, and records the arguments.
type fakeProvider struct {
	mu      sync.Mutex
	calls   []shareCall
	handle  map[string]func(args map[string]any) (invoke.Result, error)
	streams map[string][]invoke.Result
}

type shareCall struct {
	invocation string
	args       map[string]any
}

func (f *fakeProvider) Invocations() []invoke.InvocationSpec {
	return []invoke.InvocationSpec{{Name: "list", Streaming: true}}
}

func (f *fakeProvider) record(name string, args map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, shareCall{name, maps.Clone(args)})
}

func (f *fakeProvider) Invoke(_ context.Context, name string, args map[string]any) (invoke.Result, error) {
	f.record(name, args)
	h, ok := f.handle[name]
	if !ok {
		return nil, fmt.Errorf("no handler for %s", name)
	}
	return h(args)
}

func (f *fakeProvider) InvokeStream(_ context.Context, name string, args map[string]any, emit invoke.StreamEmit) error {
	f.record(name, args)
	for _, r := range f.streams[name] {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// callsOf returns the arguments of each call of the invocation.
func (f *fakeProvider) callsOf(invocation string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, c := range f.calls {
		if c.invocation == invocation {
			out = append(out, c.args)
		}
	}
	return out
}

type shareFleet struct {
	shares, links, spaces *fakeProvider
	client                adminpb.SharesAdminClient
	logs                  *logBuffer
}

var (
	fleetOnce     sync.Once
	fleetFakes    [3]*fakeProvider
	fleetCtrlAddr string
)

// startShareFleet serves the fake providers over a real control channel, and
// the SharesAdmin service on a gRPC listener. The providers are registered
// once in the process, and each test resets them.
func startShareFleet(t *testing.T) *shareFleet {
	t.Helper()
	fleetOnce.Do(func() {
		// The control channel outlives the test that starts it, as the
		// registry does.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ctrl := grpc.NewServer()
		controlpb.RegisterControlServer(ctrl, control.NewServer())
		go func() { _ = ctrl.Serve(ln) }()
		fleetCtrlAddr = ln.Addr().String()
		for i, name := range []string{userShareService, publicShareService, spacesRegistry} {
			fleetFakes[i] = &fakeProvider{}
			invoke.RegisterInstance(fleetCtrlAddr+"/"+name, name, nil, fleetFakes[i], nil)
		}
	})
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	service.SetGlobalRegistry(reg)
	// Another test can have added nodes of the same services, without a
	// control channel.
	for _, name := range []string{userShareService, publicShareService, spacesRegistry} {
		if err := service.GlobalRegistry().Remove(registry.NewService(name, nil)); err != nil {
			t.Fatal(err)
		}
		if err := service.GlobalRegistry().Add(registry.NewService(name, []registry.Node{
			registry.NewNode(fleetCtrlAddr+"/"+name, fleetCtrlAddr, map[string]string{
				registry.MetaState: registry.StateReady, registry.MetaControl: fleetCtrlAddr,
			}),
		})); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range fleetFakes {
		f.mu.Lock()
		f.calls, f.handle, f.streams = nil, map[string]func(map[string]any) (invoke.Result, error){}, map[string][]invoke.Result{}
		f.mu.Unlock()
	}

	logs := &logBuffer{}
	log := zerolog.New(logs)
	s, _ := testService(t)
	srv := grpc.NewServer(grpc.UnaryInterceptor(withLogger(&log)), grpc.StreamInterceptor(
		func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			return h(srv, loggedStream{ss, appctx.WithLogger(ss.Context(), &log)})
		}))
	s.register(srv)
	conn, err := grpc.NewClient(serveGRPC(t, "tcp", "127.0.0.1:0", srv), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &shareFleet{shares: fleetFakes[0], links: fleetFakes[1], spaces: fleetFakes[2], client: adminpb.NewSharesAdminClient(conn), logs: logs}
}

type loggedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s loggedStream) Context() context.Context { return s.ctx }

func share(id, path string, orphan bool, extra map[string]string, exp *typespb.Timestamp) invoke.Result {
	r, err := invoke.ProtoResult(&adminpb.AdminShare{
		Share:  &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: id}, Expiration: exp},
		Orphan: orphan, Path: path, Extra: extra,
	})
	if err != nil {
		panic(err)
	}
	return r
}

func publicLink(id, path string, exp *typespb.Timestamp) invoke.Result {
	r, err := invoke.ProtoResult(&adminpb.AdminPublicShare{
		PublicShare: &link.PublicShare{Id: &link.PublicShareId{OpaqueId: id}, Expiration: exp}, Path: path,
	})
	if err != nil {
		panic(err)
	}
	return r
}

func recv[M any](t *testing.T, stream grpc.ServerStreamingClient[M]) ([]*M, error) {
	t.Helper()
	var out []*M
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
}

// A share or a link that does not exist is NotFound for the client, so that
// the owner fallback of the CLI can tell it from a failure.
func TestSharesAdminNotFound(t *testing.T) {
	f := startShareFleet(t)
	f.shares.handle["get"] = func(args map[string]any) (invoke.Result, error) {
		return nil, fmt.Errorf("share %s: %w", args["id"], errtypes.NotFound("share"))
	}
	f.links.handle["get"] = func(map[string]any) (invoke.Result, error) {
		return nil, fmt.Errorf("public link: %w", errtypes.NotFound("link"))
	}
	ctx := context.Background()
	if _, err := f.client.GetShare(ctx, &adminpb.GetShareRequest{Id: "4711"}); status.Code(err) != codes.NotFound {
		t.Errorf("GetShare: got %v, want NotFound", err)
	}
	ref := &adminpb.PublicLinkRef{Ref: &adminpb.PublicLinkRef_Token{Token: "tok"}}
	if _, err := f.client.GetPublicLink(ctx, &adminpb.GetPublicLinkRequest{Ref: ref}); status.Code(err) != codes.NotFound {
		t.Errorf("GetPublicLink: got %v, want NotFound", err)
	}
	if _, err := f.client.GetShare(ctx, &adminpb.GetShareRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetShare without id: got %v, want InvalidArgument", err)
	}
}

func TestGetShareDecodesTheResult(t *testing.T) {
	f := startShareFleet(t)
	f.shares.handle["get"] = func(map[string]any) (invoke.Result, error) {
		return share("4711", "/eos/user/a/alice/doc", true, map[string]string{"initial_path": "/eos/old"}, nil), nil
	}
	got, err := f.client.GetShare(context.Background(), &adminpb.GetShareRequest{Id: "4711"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetShare().GetId().GetOpaqueId() != "4711" || !got.GetOrphan() || got.GetPath() != "/eos/user/a/alice/doc" || got.GetExtra()["initial_path"] != "/eos/old" {
		t.Fatalf("unexpected share %v", got)
	}
}

// The password goes to the link provider, and never to the audit. The token of
// a link opens it, so the audit does not have it either.
func TestUpdatePublicLinkKeepsSecretsOutOfTheAudit(t *testing.T) {
	f := startShareFleet(t)
	// The link provider writes the snake_case names of AdminPublicShare.
	f.links.handle["update"] = func(map[string]any) (invoke.Result, error) {
		return invoke.Result{"public_share": map[string]any{"id": map[string]any{"opaque_id": "7"}}, "password_protected": true}, nil
	}
	password := "s3cret-pass"
	got, err := f.client.UpdatePublicLink(context.Background(), &adminpb.UpdatePublicLinkRequest{
		Ref:        &adminpb.PublicLinkRef{Ref: &adminpb.PublicLinkRef_Token{Token: "tok-abc"}},
		Expiration: &typespb.Timestamp{Seconds: uint64(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC).Unix())},
		Password:   &password,
		Fallback:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetPasswordProtected() || got.GetPublicShare().GetId().GetOpaqueId() != "7" {
		t.Fatalf("unexpected link %v", got)
	}
	calls := f.links.callsOf("update")
	if len(calls) != 1 {
		t.Fatalf("%d update calls, want 1", len(calls))
	}
	want := map[string]any{"token": "tok-abc", "password": password, "expires_at": "2027-01-02T00:00:00Z", "fallback": "true"}
	if !maps.Equal(calls[0], want) {
		t.Errorf("update args %v, want %v", calls[0], want)
	}

	lines := f.logs.audits(t, "update_public_link")
	if len(lines) != 1 {
		t.Fatalf("%d audit lines, want 1", len(lines))
	}
	if lines[0]["target"] != "link:7" || lines[0]["fallback"] != "true" || lines[0]["password"] != "set" {
		t.Errorf("unexpected audit line %v", lines[0])
	}
	f.logs.mu.Lock()
	all := f.logs.buf.String()
	f.logs.mu.Unlock()
	for _, secret := range []string{password, "tok-abc"} {
		if strings.Contains(all, secret) {
			t.Errorf("the logs have %q", secret)
		}
	}
}

// A space given by name is resolved to the space id that the share table
// keeps, and the ONLY selections are applied to what the provider sends.
func TestListSharesResolvesTheSpaceAndSelects(t *testing.T) {
	f := startShareFleet(t)
	f.spaces.handle["get"] = func(map[string]any) (invoke.Result, error) {
		sp, err := invoke.ProtoResult(&provider.StorageSpace{Id: &provider.StorageSpaceId{OpaqueId: "eosproject$c3BhY2U"}})
		return invoke.Result{"space": sp, "status": "active"}, err
	}
	f.shares.streams["list"] = []invoke.Result{
		share("1", "", true, nil, nil),
		share("2", "", false, nil, nil),
		share("3", "", true, nil, nil),
	}
	stream, err := f.client.ListShares(context.Background(), &adminpb.ListSharesRequest{
		Space:   &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: "cernbox"}},
		Orphans: adminpb.ShareSelection_SHARE_SELECTION_ONLY,
		Expired: adminpb.ShareSelection_SHARE_SELECTION_EXCLUDE,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := recv(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range got {
		ids = append(ids, m.GetShare().GetId().GetOpaqueId())
	}
	if !slices.Equal(ids, []string{"1", "3"}) {
		t.Errorf("got shares %v, want the orphans 1 and 3", ids)
	}
	calls := f.shares.callsOf("list")
	want := map[string]any{"space": "c3BhY2U", "include_orphans": "true", "include_expired": "false"}
	if len(calls) != 1 || !maps.Equal(calls[0], want) {
		t.Errorf("list args %v, want %v", calls, want)
	}
	if got := f.spaces.callsOf("get"); len(got) != 1 || got[0]["name"] != "cernbox" {
		t.Errorf("spacesregistry get args %v", got)
	}
}

func TestListPublicLinksOnlyExpired(t *testing.T) {
	f := startShareFleet(t)
	past := &typespb.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
	future := &typespb.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}
	f.links.streams["list"] = []invoke.Result{publicLink("1", "", past), publicLink("2", "", future), publicLink("3", "", nil)}
	stream, err := f.client.ListPublicLinks(context.Background(), &adminpb.ListPublicLinksRequest{
		Owner: "alice", Expired: adminpb.ShareSelection_SHARE_SELECTION_ONLY,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := recv(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GetPublicShare().GetId().GetOpaqueId() != "1" {
		t.Errorf("got %v, want link 1 only", got)
	}
}

// A space given by path is the catalogued space that holds it.
func TestListSharesResolvesASpacePath(t *testing.T) {
	f := startShareFleet(t)
	space := func(id, root string) invoke.Result {
		sp, _ := invoke.ProtoResult(&provider.StorageSpace{
			Id:       &provider.StorageSpaceId{OpaqueId: id},
			RootInfo: &provider.ResourceInfo{Path: root},
		})
		return invoke.Result{"space": sp, "status": "active"}
	}
	f.spaces.streams["list"] = []invoke.Result{
		space("eosproject$b3RoZXI", "/eos/project/o"),
		space("eosproject$c3BhY2U", "/eos/project/c"),
	}
	stream, err := f.client.ListShares(context.Background(), &adminpb.ListSharesRequest{
		Space: &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: "/eos/project/c/docs"}},
	})
	if err == nil {
		_, err = recv(t, stream)
	}
	if err != nil {
		t.Fatal(err)
	}
	if calls := f.shares.callsOf("list"); len(calls) != 1 || calls[0]["space"] != "c3BhY2U" {
		t.Errorf("list args %v, want space c3BhY2U", calls)
	}

	f.spaces.streams["list"] = nil
	stream, err = f.client.ListShares(context.Background(), &adminpb.ListSharesRequest{
		Space: &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: "/eos/user/a"}},
	})
	if err == nil {
		_, err = recv(t, stream)
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("a path in no catalogued space: got %v, want NotFound", err)
	}
}

func TestSharesAdminRefusesBadRequests(t *testing.T) {
	f := startShareFleet(t)
	ctx := context.Background()
	move, err := f.client.MoveShares(ctx, &adminpb.MoveSharesRequest{FromPath: "/eos/a", ToPath: "/eos/b"})
	if err == nil {
		_, err = recv(t, move)
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("MoveShares without owner or space: got %v, want InvalidArgument", err)
	}
	if _, err := f.client.UpdateShare(ctx, &adminpb.UpdateShareRequest{Id: "1", Expiration: &typespb.Timestamp{Seconds: 1}, RemoveExpiration: true}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("UpdateShare with two expirations: got %v, want InvalidArgument", err)
	}
	if n := len(f.shares.callsOf("list")) + len(f.shares.callsOf("update")); n != 0 {
		t.Errorf("the provider was called %d times", n)
	}
}

// MoveShares moves the shares and the links under from_path, found by their
// current path or by their path at creation, and keeps going after a failure.
func TestMoveShares(t *testing.T) {
	f := startShareFleet(t)
	f.shares.streams["list"] = []invoke.Result{
		share("1", "/eos/a/x", false, nil, nil),
		share("2", "", false, map[string]string{"initial_path": "/eos/a/y/z"}, nil),
		share("3", "/eos/ab", false, nil, nil),
		share("4", "/eos/c", false, map[string]string{"initial_path": "/eos/a/w"}, nil),
	}
	f.links.streams["list"] = []invoke.Result{publicLink("9", "/eos/a", nil)}
	f.shares.handle["move"] = func(args map[string]any) (invoke.Result, error) {
		if args["id"] == "2" {
			return nil, fmt.Errorf("moving share 2: %w", errtypes.NotFound("/eos/b/y/z"))
		}
		return share(args["id"].(string), args["path"].(string), false, nil, nil), nil
	}
	f.links.handle["move"] = func(args map[string]any) (invoke.Result, error) {
		return publicLink(args["id"].(string), args["path"].(string), nil), nil
	}

	req := &adminpb.MoveSharesRequest{FromPath: "/eos/a/", ToPath: "/eos/b", Owner: "alice", As: "bob", ApplyGrant: true, DryRun: true}
	type moved struct{ id, from, to, err string }
	run := func() ([]moved, error) {
		stream, err := f.client.MoveShares(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		items, err := recv(t, stream)
		var out []moved
		for _, it := range items {
			id := it.GetShare().GetShare().GetId().GetOpaqueId() + it.GetPublicShare().GetPublicShare().GetId().GetOpaqueId()
			out = append(out, moved{id, it.GetFromPath(), it.GetToPath(), it.GetError()})
		}
		return out, err
	}

	got, err := run()
	if err != nil {
		t.Fatal(err)
	}
	want := []moved{{"1", "/eos/a/x", "/eos/b/x", ""}, {"2", "/eos/a/y/z", "/eos/b/y/z", ""}, {"9", "/eos/a", "/eos/b", ""}}
	if !slices.Equal(got, want) {
		t.Fatalf("dry run: got %v, want %v", got, want)
	}
	if n := len(f.shares.callsOf("move")) + len(f.links.callsOf("move")); n != 0 {
		t.Fatalf("the dry run moved %d items", n)
	}
	if calls := f.shares.callsOf("list"); calls[0]["as"] != "bob" || calls[0]["owner"] != "alice" || calls[0]["resolve_paths"] != "true" {
		t.Errorf("share list args %v", calls[0])
	}

	req.DryRun = false
	got, err = run()
	if status.Code(err) != codes.NotFound || !strings.Contains(err.Error(), "1 of 3") {
		t.Errorf("got %v, want NotFound for 1 of 3 moves", err)
	}
	if len(got) != 3 || got[0].err != "" || got[1].err == "" || got[2].err != "" {
		t.Errorf("unexpected items %v", got)
	}
	shareMoves := f.shares.callsOf("move")
	if len(shareMoves) != 2 || !maps.Equal(shareMoves[0], map[string]any{"id": "1", "path": "/eos/b/x", "as": "bob", "apply_grant": "true"}) {
		t.Errorf("share moves %v", shareMoves)
	}
	if linkMoves := f.links.callsOf("move"); len(linkMoves) != 1 || !maps.Equal(linkMoves[0], map[string]any{"id": "9", "path": "/eos/b", "as": "bob"}) {
		t.Errorf("link moves %v", linkMoves)
	}
	if lines := f.logs.audits(t, "move_shares"); len(lines) != 1 || lines[0]["moved"] != "2" || lines[0]["failed"] != "1" {
		t.Errorf("move audit %v", lines)
	}
}
