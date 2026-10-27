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
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type quotaCall struct {
	node, invocation string
	args             map[string]string
}

// Like the storage provider, an instance uses the type of its spaces when the call gives none.
type fakeQuotaProviders struct {
	mu    sync.Mutex
	calls []quotaCall
}

func (f *fakeQuotaProviders) set(node, mountID, provides string) *invoke.Set {
	type quotaArgs struct {
		Root      string       `arg:"root,required"`
		SpaceType string       `arg:"space_type"`
		Owner     admin.UserID `arg:"owner"`
		MaxBytes  *uint64      `arg:"max_bytes"`
		MaxFiles  *uint64      `arg:"max_files"`
	}
	type listArgs struct {
		MinBytes string `arg:"min_bytes"`
	}
	record := func(invocation string, args map[string]string) {
		maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, quotaCall{node: node, invocation: invocation, args: args})
	}
	result := func(a quotaArgs, maxBytes uint64) invoke.Result {
		t := cmp.Or(a.SpaceType, provides)
		form := "user"
		if t == string(spaces.SpaceTypeProject) {
			form = "project"
		}
		r := invoke.Result{"root": a.Root, "mount_id": mountID, "max_bytes": maxBytes, "used_bytes": 400, "physical_max_bytes": 2 * maxBytes, "form": form, "space_type": t}
		if id := a.Owner.ID(); id != nil {
			owner, _ := invoke.ProtoResult(id)
			r["owner"] = map[string]any(owner)
		}
		return r
	}
	set := invoke.NewSet()
	set.Add("quota_get", "").Handle(invoke.Typed(func(_ context.Context, a quotaArgs) (invoke.Result, error) {
		record("quota_get", map[string]string{"root": a.Root, "space_type": a.SpaceType, "owner": admin.UserArg(a.Owner.ID())})
		return result(a, 1000), nil
	}))
	set.Add("quota_set", "").Mutating().Handle(invoke.Typed(func(_ context.Context, a quotaArgs) (invoke.Result, error) {
		args := map[string]string{"root": a.Root, "space_type": a.SpaceType, "owner": admin.UserArg(a.Owner.ID())}
		if a.MaxBytes != nil {
			args["max_bytes"] = strconv.FormatUint(*a.MaxBytes, 10)
		}
		if a.MaxFiles != nil {
			args["max_files"] = strconv.FormatUint(*a.MaxFiles, 10)
		}
		record("quota_set", args)
		return result(a, *a.MaxBytes), nil
	}))
	set.Add("quota_list", "").HandleStream(invoke.TypedStream(func(_ context.Context, a listArgs, emit invoke.StreamEmit) error {
		record("quota_list", map[string]string{"min_bytes": a.MinBytes})
		for _, r := range []string{"x", "y"} {
			if err := emit(invoke.Result{"root": "/" + mountID + "/" + r, "mount_id": mountID, "max_bytes": 10, "used_bytes": 30, "max_files": 5, "used_files": 2}); err != nil {
				return err
			}
		}
		return nil
	}))
	return set
}

func (f *fakeQuotaProviders) callsOf(invocation string) []quotaCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []quotaCall
	for _, c := range f.calls {
		if c.invocation == invocation {
			out = append(out, c)
		}
	}
	return out
}

// An OCM storage provider has no quota invocations, and one instance is offline.
func quotaFleet(t *testing.T) (adminpb.SpacesAdminClient, *fakeQuotaProviders) {
	t.Helper()
	c, cat := spacesClient(t)
	cat.spaces = append(cat.spaces, &adminpb.AdminSpace{Status: "active", Space: &provider.StorageSpace{
		Id:        &provider.StorageSpaceId{OpaqueId: "eosproject-c$cms"},
		Name:      "cms",
		SpaceType: string(spaces.SpaceTypeProject),
		Owner:     &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "cmsadm"}},
		RootInfo:  &provider.ResourceInfo{Path: "/eos/project/c/cms"},
	}})

	srv := grpc.NewServer()
	controlpb.RegisterControlServer(srv, control.NewServer())
	ctrl := serveGRPC(t, "tcp", "127.0.0.1:0", srv)
	reg := service.GlobalRegistry()
	add := func(name string, nodes ...registry.Node) {
		if err := reg.Add(registry.NewService(name, nodes)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reg.Remove(registry.NewService(name, nil)) })
	}
	node := func(id, svc string, set *invoke.Set, meta map[string]string) registry.Node {
		invoke.RegisterInstance(id, svc, nil, set, nil)
		md := invoke.Metadata(id)
		md[registry.MetaState] = registry.StateReady
		md[registry.MetaControl] = ctrl
		maps.Copy(md, meta)
		return registry.NewNode(id, id, md)
	}

	table := &routeTable{routes: map[string]string{"/eos/user/a": "eoshome-a", "/eos/project/c": "eosproject-c"}}
	sr := &fakeStorageRegistry{table: table, live: maps.Clone(table.routes)}
	add(storageRegistryService, node("reg1:9999/storageregistry", storageRegistryService, sr.set(), nil))

	f := &fakeQuotaProviders{}
	sp := func(id, mountID, provides, state string) registry.Node {
		return node(id, storageProviderService, f.set(id, mountID, provides), map[string]string{registry.MetaMountID: mountID, registry.MetaState: state})
	}
	add(storageProviderService,
		sp("home-a1:9001/storageprovider", "eoshome-a", "personal", registry.StateReady),
		sp("home-a2:9001/storageprovider", "eoshome-a", "personal", registry.StateReady),
		sp("home-a3:9001/storageprovider", "eoshome-a", "personal", registry.StateOffline),
		sp("proj-c1:9001/storageprovider", "eosproject-c", "project", registry.StateReady),
		// No control channel: a call to it fails, so the list must skip it.
		registry.NewNode("ocm:9001/storageprovider", "ocm:9001", map[string]string{registry.MetaMountID: "ocm"}),
	)
	return c, f
}

func byID(id string) *adminpb.SpaceRef {
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: &provider.StorageSpaceId{OpaqueId: id}}}
}

// A home is not in the catalogue: it is known by its root, and its instance chooses the type.
func TestGetSpaceQuotaFindsTheSpaceAndItsInstance(t *testing.T) {
	c, f := quotaFleet(t)
	ctx := context.Background()

	q, err := c.GetSpaceQuota(ctx, &adminpb.GetSpaceQuotaRequest{Ref: byName("cms")})
	if err != nil {
		t.Fatal(err)
	}
	if q.Form != "project" || q.MountId != "eosproject-c" || q.Space.GetId().GetOpaqueId() != "eosproject-c$cms" || q.PhysicalMaxBytes != 2000 || q.Owner.GetIdp() != "cern" {
		t.Errorf("project quota = %v", q)
	}
	if q.Quota.GetQuotaMaxBytes() != 1000 || q.Quota.GetRemainingBytes() != 600 || q.UsedBytes != 400 {
		t.Errorf("quota = %v, want 600 of 1000 bytes remaining", q.Quota)
	}
	want := quotaCall{node: "proj-c1:9001/storageprovider", invocation: "quota_get",
		args: map[string]string{"root": "/eos/project/c/cms", "space_type": "project", "owner": admin.UserArg(&userpb.UserId{Idp: "cern", OpaqueId: "cmsadm"})}}
	if calls := f.callsOf("quota_get"); len(calls) != 1 || calls[0].node != want.node || !maps.Equal(calls[0].args, want.args) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}

	home := "/eos/user/a/alice"
	for _, ref := range []*adminpb.SpaceRef{byPath(home), byID(spaces.EncodeSpaceID(home)), byID("eoshome-a$" + spaces.EncodeSpaceID(home))} {
		q, err := c.GetSpaceQuota(ctx, &adminpb.GetSpaceQuotaRequest{Ref: ref})
		if err != nil {
			t.Fatalf("%v: %v", ref, err)
		}
		if q.Root != home || q.Form != "user" || q.SpaceType != "personal" || q.MountId != "eoshome-a" || q.Space != nil || q.Owner != nil {
			t.Errorf("%v: home quota = %v", ref, q)
		}
	}
	for _, call := range f.callsOf("quota_get")[1:] {
		if !maps.Equal(call.args, map[string]string{"root": home}) || (call.node != "home-a1:9001/storageprovider" && call.node != "home-a2:9001/storageprovider") {
			t.Errorf("home call = %v", call)
		}
	}

	for _, ref := range []*adminpb.SpaceRef{byName("nope"), byPath("/eos/media/x"), byID("not-a-space-id")} {
		if _, err := c.GetSpaceQuota(ctx, &adminpb.GetSpaceQuotaRequest{Ref: ref}); status.Code(err) != codes.NotFound {
			t.Errorf("%v: got %v, want NotFound", ref, err)
		}
	}
}

func TestSetSpaceQuota(t *testing.T) {
	c, f := quotaFleet(t)
	ctx := context.Background()
	limits := &provider.Quota{QuotaMaxBytes: 5000}
	for _, mask := range []*fieldmaskpb.FieldMask{nil, {Paths: []string{"remaining_bytes"}}} {
		if _, err := c.SetSpaceQuota(ctx, &adminpb.SetSpaceQuotaRequest{Ref: byName("cms"), Quota: limits, UpdateMask: mask}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("mask %v: got %v, want InvalidArgument", mask, err)
		}
	}
	q, err := c.SetSpaceQuota(ctx, &adminpb.SetSpaceQuotaRequest{Ref: byPath("/eos/project/c/cms/some/folder"), Quota: limits, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"quota_max_bytes"}}})
	if err != nil {
		t.Fatal(err)
	}
	if q.Quota.GetQuotaMaxBytes() != 5000 || q.Quota.GetRemainingBytes() != 4600 || q.Root != "/eos/project/c/cms" {
		t.Errorf("quota = %v", q)
	}
	// A limit that the mask does not name is not sent.
	calls := f.callsOf("quota_set")
	if len(calls) != 1 || calls[0].args["root"] != "/eos/project/c/cms" {
		t.Fatalf("calls = %v", calls)
	}
	if _, ok := calls[0].args["max_files"]; ok || calls[0].args["max_bytes"] != "5000" {
		t.Errorf("quota_set args = %v", calls[0].args)
	}
}

func TestListSpaceQuotasAsksOneInstanceOfEachMount(t *testing.T) {
	c, f := quotaFleet(t)
	stream, err := c.ListSpaceQuotas(context.Background(), &adminpb.ListSpaceQuotasRequest{MinBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	qs, err := drain(t, stream.Recv)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, q := range qs {
		got = append(got, q.MountId+":"+q.Root)
		// Over the limit, nothing remains, and the usage is still known.
		if q.Quota.GetRemainingBytes() != 0 || q.Quota.GetRemainingFiles() != 3 || q.UsedBytes != 30 || q.UsedFiles != 2 {
			t.Errorf("%s: quota = %v", q.Root, q.Quota)
		}
	}
	slices.Sort(got)
	want := []string{"eoshome-a:/eoshome-a/x", "eoshome-a:/eoshome-a/y", "eosproject-c:/eosproject-c/x", "eosproject-c:/eosproject-c/y"}
	if !slices.Equal(got, want) {
		t.Errorf("quotas = %v, want %v", got, want)
	}
	calls := f.callsOf("quota_list")
	if len(calls) != 2 {
		t.Fatalf("quota_list ran %d times, want once for each mount: %v", len(calls), calls)
	}
	for _, call := range calls {
		if call.args["min_bytes"] != "5" {
			t.Errorf("min_bytes not sent: %v", call)
		}
	}
}
