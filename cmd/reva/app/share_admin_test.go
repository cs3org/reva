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

package revacmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type sharesFleet struct {
	fakeAdminAPI
	adminpb.UnimplementedSharesAdminServer

	mu       sync.Mutex
	requests []proto.Message
}

func (f *sharesFleet) record(m proto.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, m)
}

func sent[M proto.Message](f *sharesFleet) []M {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []M
	for _, r := range f.requests {
		if m, ok := r.(M); ok {
			out = append(out, m)
		}
	}
	return out
}

var (
	bobShare = &adminpb.AdminShare{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "4711"}, Owner: &userpb.UserId{OpaqueId: "bob"}}}
	bobLink  = &adminpb.AdminPublicShare{PublicShare: &link.PublicShare{Id: &link.PublicShareId{OpaqueId: "42"}, Token: "tok", Owner: &userpb.UserId{OpaqueId: "bob"}}}
)

func (f *sharesFleet) share(ref *collaboration.ShareReference) (*adminpb.AdminShare, error) {
	if ref.GetId().GetOpaqueId() != "4711" {
		return nil, status.Error(codes.NotFound, "share not found")
	}
	return bobShare, nil
}

func (f *sharesFleet) link(ref *link.PublicShareReference) (*adminpb.AdminPublicShare, error) {
	if ref.GetId().GetOpaqueId() != "42" && ref.GetToken() != "tok" {
		return nil, status.Error(codes.NotFound, "link not found")
	}
	return bobLink, nil
}

func (f *sharesFleet) GetShare(_ context.Context, r *adminpb.GetShareRequest) (*adminpb.AdminShare, error) {
	f.record(r)
	return f.share(r.Ref)
}

func (f *sharesFleet) UpdateShare(_ context.Context, r *adminpb.UpdateShareRequest) (*adminpb.AdminShare, error) {
	f.record(r)
	return f.share(r.Ref)
}

func (f *sharesFleet) RemoveShare(_ context.Context, r *adminpb.RemoveShareRequest) (*adminpb.AdminShare, error) {
	f.record(r)
	return f.share(r.Ref)
}

func (f *sharesFleet) TransferShare(_ context.Context, r *adminpb.TransferShareRequest) (*adminpb.AdminShare, error) {
	f.record(r)
	return f.share(r.Ref)
}

func (f *sharesFleet) ListShares(r *adminpb.ListSharesRequest, s adminpb.SharesAdmin_ListSharesServer) error {
	f.record(r)
	return s.Send(&adminpb.AdminShare{Share: bobShare.Share, Orphan: true, Path: "/eos/user/b/bob/old"})
}

func (f *sharesFleet) MoveShares(r *adminpb.MoveSharesRequest, s adminpb.SharesAdmin_MoveSharesServer) error {
	f.record(r)
	return s.Send(&adminpb.MovedShare{Item: &adminpb.MovedShare_Share{Share: bobShare}, FromPath: r.FromPath + "/x", ToPath: r.ToPath + "/x"})
}

func (f *sharesFleet) GetPublicLink(_ context.Context, r *adminpb.GetPublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	f.record(r)
	return f.link(r.Ref)
}

func (f *sharesFleet) UpdatePublicLink(_ context.Context, r *adminpb.UpdatePublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	f.record(r)
	return f.link(r.Ref)
}

func (f *sharesFleet) RemovePublicLink(_ context.Context, r *adminpb.RemovePublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	f.record(r)
	return f.link(r.Ref)
}

func (f *sharesFleet) MarkPublicLinkOrphan(_ context.Context, r *adminpb.MarkPublicLinkOrphanRequest) (*adminpb.AdminPublicShare, error) {
	f.record(r)
	return f.link(r.Ref)
}

func (f *sharesFleet) ListPublicLinks(r *adminpb.ListPublicLinksRequest, s adminpb.SharesAdmin_ListPublicLinksServer) error {
	f.record(r)
	return s.Send(bobLink)
}

func runAdmin(t *testing.T, b backend.Backend, f *sharesFleet, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(srv, f)
	adminpb.RegisterSharesAdminServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	rt := &cli.Runtime{}
	rt.UseBackend(b, nil)
	defer rt.Close()
	root := rootCommand(rt, new(bool))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"--admin-host", lis.Addr().String(), "--insecure", "--token", "login", "--yes"}, args...))
	err = root.Execute()
	return out.String(), err
}

func TestOwnerFallback(t *testing.T) {
	b := clitest.New("alice")
	f := &sharesFleet{}

	out, err := runAdmin(t, b, f, "", "share", "get", "4711")
	if err != nil || !strings.Contains(out, "bob") {
		t.Errorf("share get = %q, %v: want the share of bob", out, err)
	}
	if _, err := runAdmin(t, b, f, "", "share", "update", "4711", "--role", "editor", "--expires", "none"); err != nil {
		t.Fatal(err)
	}
	editor := &collaboration.UpdateShareRequest_UpdateField{Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
		Permissions: &collaboration.SharePermissions{Permissions: permissions.NewEditorRole().CS3ResourcePermissions()},
	}}
	u := sent[*adminpb.UpdateShareRequest](f)
	if len(u) != 1 || u[0].GetRef().GetId().GetOpaqueId() != "4711" || len(u[0].Fields) != 2 || !proto.Equal(u[0].Fields[0], editor) ||
		u[0].Fields[1].GetExpiration().GetSeconds() != 0 || !u[0].Fallback {
		t.Errorf("UpdateShare requests %v", u)
	}
	if _, err := runAdmin(t, b, f, "", "share", "remove", "4711"); err != nil {
		t.Fatal(err)
	}
	if r := sent[*adminpb.RemoveShareRequest](f); len(r) != 1 || r[0].DbOnly || !r[0].Fallback {
		t.Errorf("RemoveShare requests %v", r)
	}

	if _, err := runAdmin(t, b, f, "s3cret\n", "link", "update", "42", "--password", "--expires", "2026-12-31"); err != nil {
		t.Fatal(err)
	}
	lu := sent[*adminpb.UpdatePublicLinkRequest](f)
	if len(lu) != 1 || lu[0].GetRef().GetId().GetOpaqueId() != "42" || len(lu[0].Updates) != 2 || !lu[0].Fallback ||
		lu[0].Updates[0].GetType() != link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION || lu[0].Updates[0].GetGrant().GetExpiration().GetSeconds() == 0 ||
		lu[0].Updates[1].GetType() != link.UpdatePublicShareRequest_Update_TYPE_PASSWORD || lu[0].Updates[1].GetGrant().GetPassword() != "s3cret" {
		t.Errorf("UpdatePublicLink requests %v", lu)
	}
	if _, err := runAdmin(t, b, f, "", "link", "update", "42", "--description", "x"); err == nil {
		t.Error("link update --description of another user's link: want an error")
	}
	if _, err := runAdmin(t, b, f, "", "link", "remove", "42"); err != nil {
		t.Fatal(err)
	}
	if r := sent[*adminpb.RemovePublicLinkRequest](f); len(r) != 1 || !r[0].Fallback {
		t.Errorf("RemovePublicLink requests %v", r)
	}
}

// A caller who cannot be an admin gets the not-found of the user plane, and
// the Admin API is not asked.
func TestOwnerFallbackKeepsTheNotFound(t *testing.T) {
	b := clitest.New("alice")
	f := &sharesFleet{notAdmin: true}
	if _, err := runAdmin(t, b, f, "", "share", "update", "4711", "--role", "editor"); !cli.IsNotFound(err) {
		t.Errorf("non-admin: err = %v, want not found", err)
	}
	if _, err := runAdmin(t, b, f, "", "link", "remove", "42"); !cli.IsNotFound(err) {
		t.Errorf("non-admin: err = %v, want not found", err)
	}
	if n := len(sent[proto.Message](f)); n != 0 {
		t.Errorf("a non-admin sent %d SharesAdmin requests", n)
	}

	f.notAdmin = false
	if _, err := runAdmin(t, b, f, "", "share", "remove", "4712"); !cli.IsNotFound(err) {
		t.Errorf("mistyped id: err = %v, want not found", err)
	}
	if n := len(sent[*adminpb.RemoveShareRequest](f)); n != 0 {
		t.Errorf("a mistyped id sent %d RemoveShare requests", n)
	}
}

// directoryUser has an opaque id that differs from its username.
func directoryUser(name string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "id-" + name, Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: name}
}

func addDirectoryUsers(b *clitest.Backend, names ...string) {
	for _, n := range names {
		b.AddUser(directoryUser(n))
	}
}

func TestShareOperatorOptions(t *testing.T) {
	b := clitest.New("alice")
	addDirectoryUsers(b, "bob", "carol")
	f := &sharesFleet{}

	out, err := runAdmin(t, b, f, "", "share", "list", "--owner", "bob", "--orphans", "--expired", "--space", "cernbox")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ORPHAN") || !strings.Contains(out, "/eos/user/b/bob/old") {
		t.Errorf("share list = %q: want the ORPHAN and PATH columns", out)
	}
	l := sent[*adminpb.ListSharesRequest](f)
	want := &adminpb.ListSharesRequest{
		Filters: []*collaboration.Filter{{Type: collaboration.Filter_TYPE_OWNER, Term: &collaboration.Filter_Owner{Owner: directoryUser("bob").Id}}},
		Space:   &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: "cernbox"}},
		Orphans: adminpb.ShareSelection_SHARE_SELECTION_ONLY, Expired: adminpb.ShareSelection_SHARE_SELECTION_ONLY, ResolvePaths: true,
	}
	if len(l) != 1 || !proto.Equal(l[0], want) {
		t.Errorf("ListShares requests %v, want %v", l, want)
	}
	if _, err := runAdmin(t, b, f, "", "share", "list", "--all"); err == nil {
		t.Error("share list --all without a filter: want an error")
	}
	if _, err := runAdmin(t, b, f, "", "share", "list", "--all", "--by-resource-id", "eoshome:100"); err != nil {
		t.Fatal(err)
	}
	resource := &provider.ResourceId{StorageId: "eoshome", OpaqueId: "100"}
	if l := sent[*adminpb.ListSharesRequest](f); len(l) != 2 || len(l[1].Filters) != 1 || !proto.Equal(l[1].Filters[0].GetResourceId(), resource) {
		t.Errorf("ListShares requests %v, want a RESOURCE_ID filter", l)
	}
	if _, err := runAdmin(t, b, f, "", "link", "list", "--space", "eosproject$c3BhY2U"); err != nil {
		t.Fatal(err)
	}
	if l := sent[*adminpb.ListPublicLinksRequest](f); len(l) != 1 || l[0].GetSpace().GetId().GetOpaqueId() != "eosproject$c3BhY2U" || len(l[0].Filters) != 0 {
		t.Errorf("ListPublicLinks requests %v", l)
	}
	if _, err := runAdmin(t, b, f, "", "link", "list", "--owner", "bob", "--by-resource-id", "eoshome:100"); err != nil {
		t.Fatal(err)
	}
	if l := sent[*adminpb.ListPublicLinksRequest](f); len(l) != 2 || len(l[1].Filters) != 2 ||
		!proto.Equal(l[1].Filters[0].GetOwner(), directoryUser("bob").Id) || !proto.Equal(l[1].Filters[1].GetResourceId(), resource) {
		t.Errorf("ListPublicLinks requests %v, want OWNER and RESOURCE_ID filters", l)
	}

	if _, err := runAdmin(t, b, f, "", "share", "remove", "4711", "--db-only"); err != nil {
		t.Fatal(err)
	}
	if r := sent[*adminpb.RemoveShareRequest](f); len(r) != 1 || !r[0].DbOnly || r[0].Fallback {
		t.Errorf("RemoveShare requests %v", r)
	}
	if _, err := runAdmin(t, b, f, "", "share", "list", "--owner", "bob", "--as", "carol"); err == nil {
		t.Error("an operator option with --as: want a usage error")
	}
}

func TestShareOperatorVerbs(t *testing.T) {
	b := clitest.New("alice")
	addDirectoryUsers(b, "bob", "carol")
	f := &sharesFleet{}
	if _, err := runAdmin(t, b, f, "", "share", "transfer", "4711", "carol"); err != nil {
		t.Fatal(err)
	}
	if r := sent[*adminpb.TransferShareRequest](f); len(r) != 1 || !proto.Equal(r[0].GetInitiator(), directoryUser("carol").Id) || r[0].GetRef().GetId().GetOpaqueId() != "4711" {
		t.Errorf("TransferShare requests %v", r)
	}
	if _, err := runAdmin(t, b, f, "", "link", "transfer", "42", "nobody"); err == nil || err.Error() != "no user nobody" {
		t.Errorf("link transfer to nobody: err = %v, want no user nobody", err)
	}
	if r := sent[*adminpb.TransferPublicLinkRequest](f); len(r) != 0 {
		t.Errorf("TransferPublicLink requests %v, want none", r)
	}
	if _, err := runAdmin(t, b, f, "", "link", "mark-orphan", "tok"); err != nil {
		t.Fatal(err)
	}
	if r := sent[*adminpb.MarkPublicLinkOrphanRequest](f); len(r) != 1 || r[0].GetRef().GetToken() != "tok" {
		t.Errorf("MarkPublicLinkOrphan requests %v", r)
	}

	out, err := runAdmin(t, b, f, "", "share", "move", "/eos/a", "/eos/b", "--owner", "bob")
	if err != nil || !strings.Contains(out, "/eos/b/x") {
		t.Fatalf("share move = %q, %v", out, err)
	}
	if m := sent[*adminpb.MoveSharesRequest](f); len(m) != 2 || !m[0].DryRun || m[1].DryRun || !proto.Equal(m[1].GetOwner(), directoryUser("bob").Id) || m[1].As != nil {
		t.Errorf("MoveShares requests %v", m)
	}
	if _, err := runAdmin(t, b, f, "", "share", "move", "/eos/a", "/eos/b"); err == nil {
		t.Error("share move without --owner or --space: want an error")
	}
}

type existsBackend struct{ *clitest.Backend }

func (b existsBackend) Shares() backend.Shares { return existsShares{b.Backend.Shares()} }

type existsShares struct{ backend.Shares }

func (s existsShares) Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error) {
	shares, err := s.List(ctx, req.ResourceInfo.Id)
	if err != nil {
		return nil, err
	}
	for _, sh := range shares {
		if granteeName(sh.Grantee) == granteeName(req.Grant.Grantee) {
			return nil, &cli.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: "share already exists"}
		}
	}
	return s.Shares.Create(ctx, req)
}

func TestShareCreateIgnoreExisting(t *testing.T) {
	b := existsBackend{clitest.New("alice")}
	b.AddFile("/docs/a.txt", []byte("x"))
	f := &sharesFleet{}
	if _, err := runAdmin(t, b, f, "", "share", "create", "/docs/a.txt", "--with", "bob"); err != nil {
		t.Fatal(err)
	}
	_, err := runAdmin(t, b, f, "", "share", "create", "/docs/a.txt", "--with", "bob")
	if se, ok := errors.AsType[*cli.StatusError](err); !ok || se.Code != rpc.Code_CODE_ALREADY_EXISTS {
		t.Errorf("a second share: err = %v, want AlreadyExists", err)
	}
	out, err := runAdmin(t, b, f, "", "share", "create", "/docs/a.txt", "--with", "bob", "--ignore-existing")
	if err != nil || !strings.Contains(out, "already has a share") || !strings.Contains(out, "bob") {
		t.Errorf("--ignore-existing: %q, %v", out, err)
	}
}
