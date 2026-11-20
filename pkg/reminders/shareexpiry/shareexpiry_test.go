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

package shareexpiry

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/reminders"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"google.golang.org/grpc"
)

// fakeAdmin answers the expiring invocation two shares at a time, and the
// space invocation from spaces.
type fakeAdmin struct {
	adminpb.AdminAPIClient
	shares []share
	spaces map[string]any
}

func (f *fakeAdmin) Invoke(_ context.Context, req *adminpb.InvokeRequest, _ ...grpc.CallOption) (*adminpb.InvokeResponse, error) {
	if req.GetInvocation() == "space" {
		b, _ := json.Marshal(f.spaces[req.GetArgs()["id"]])
		return &adminpb.InvokeResponse{Results: []*adminpb.NodeResult{{Node: "up", ResultJson: string(b)}}}, nil
	}
	start := 0
	if id := req.GetArgs()["after_id"]; id != "" {
		n, _ := strconv.Atoi(id)
		start = slices.IndexFunc(f.shares, func(s share) bool { return s.ID == uint(n) }) + 1
	}
	page := f.shares[start:min(start+2, len(f.shares))]
	b, _ := json.Marshal(map[string]any{"shares": page})
	return &adminpb.InvokeResponse{Results: []*adminpb.NodeResult{{Node: "down", Error: "unavailable"}, {Node: "up", ResultJson: string(b)}}}, nil
}

type fakeGateway struct {
	gateway.GatewayAPIClient
	paths map[string]string
}

// Stat puts a path under /eos/project/ in the space project-<initial>.
func (g *fakeGateway) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	switch p := g.paths[req.GetRef().GetResourceId().GetOpaqueId()]; p {
	case "":
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	case "error":
		return nil, errors.New("storage down")
	default:
		space := &provider.StorageSpace{SpaceType: "personal"}
		if rest, ok := strings.CutPrefix(p, "/eos/project/"); ok {
			space = &provider.StorageSpace{SpaceType: "project", Id: &provider.StorageSpaceId{OpaqueId: "project-" + rest[:1]}}
		}
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: &provider.ResourceInfo{Path: p, Space: space}}, nil
	}
}

func newKind(t *testing.T, conf map[string]any, shares []share, paths map[string]string) *kind {
	t.Helper()
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "shareexpiry-test"})
	if sharedconf.GetJWTSecret("") == "" {
		t.Skip("shared config initialised without a secret")
	}
	k, err := New(context.Background(), conf)
	if err != nil {
		t.Fatal(err)
	}
	kk := k.(*kind)
	kk.admin = &fakeAdmin{shares: shares, spaces: map[string]any{
		"project-c": map[string]any{"name": "cernbox", "roles": []map[string]any{
			{"name": "reader", "groups": []string{"cernbox-readers"}},
			{"name": "admin", "users": []string{"alice"}, "groups": []string{"cernbox-admins"}},
		}},
	}}
	kk.gateway = &fakeGateway{paths: paths}
	kk.now = func() time.Time { return time.Date(2026, 10, 16, 8, 0, 0, 0, time.Local) }
	return kk
}

func day(d int) time.Time { return time.Date(2026, 10, d, 10, 0, 0, 0, time.Local) }

type got struct {
	subject string
	level   int
	to      reminders.Recipient
}

func scan(k *kind) ([]got, int) {
	var items []got
	var errs int
	for it, err := range k.Scan(context.Background()) {
		if err != nil {
			errs++
			continue
		}
		items = append(items, got{it.Subject, it.Level, it.Recipient})
	}
	return items, errs
}

func TestScan(t *testing.T) {
	old := day(1)
	shares := []share{
		{ID: 1, Initiator: "einstein", ShareWith: "marie", Inode: "1", ExpiresAt: day(17), UpdatedAt: old},
		{ID: 2, Initiator: "einstein", ShareWith: "physics", Group: true, Inode: "2", ExpiresAt: day(23), UpdatedAt: old},
		{ID: 3, Initiator: "marie", ShareWith: "bob", Inode: "3", ExpiresAt: day(20), UpdatedAt: old},
		// created with 3 days left: the 7-day point is skipped
		{ID: 4, Initiator: "marie", ShareWith: "bob", Inode: "4", ExpiresAt: day(19), UpdatedAt: day(16)},
		{ID: 5, Initiator: "marie", ShareWith: "bob", Inode: "5", ExpiresAt: day(19), UpdatedAt: old},
		{ID: 6, Initiator: "marie", ShareWith: "bob", Inode: "6", ExpiresAt: day(19), UpdatedAt: old},
	}
	paths := map[string]string{
		"1": "/eos/user/e/einstein/a",
		"2": "/eos/user/e/einstein/b",
		"3": "/eos/project/c/cernbox/docs",
		"4": "/eos/user/m/marie/d",
		"6": "error",
	}

	items, errs := scan(newKind(t, nil, shares, paths))
	want := []got{
		{"share:2026-10-17:1:sharer:einstein", 2, reminders.Recipient{Name: "einstein"}},
		{"share:2026-10-23:2:sharer:einstein", 1, reminders.Recipient{Name: "einstein"}},
		{"share:2026-10-20:3:sharer:alice", 1, reminders.Recipient{Name: "alice"}},
		{"share:2026-10-20:3:sharer:cernbox-admins", 1, reminders.Recipient{Name: "cernbox-admins", Group: true}},
		{"share:2026-10-19:4:sharer:marie", 0, reminders.Recipient{Name: "marie"}},
	}
	if !slices.Equal(items, want) || errs != 1 {
		t.Fatalf("got %+v and %d errors, want %+v and 1 error", items, errs, want)
	}

	items, _ = scan(newKind(t, map[string]any{"notify": []string{"grantee"}}, shares[:3], paths))
	want = []got{
		{"share:2026-10-17:1:grantee:marie", 2, reminders.Recipient{Name: "marie"}},
		{"share:2026-10-23:2:grantee:physics", 1, reminders.Recipient{Name: "physics", Group: true}},
		{"share:2026-10-20:3:grantee:bob", 1, reminders.Recipient{Name: "bob"}},
	}
	if !slices.Equal(items, want) {
		t.Fatalf("grantee: got %+v, want %+v", items, want)
	}
}

func TestNewRefusesUnknownNotify(t *testing.T) {
	if _, err := New(context.Background(), map[string]any{"notify": []string{"owner"}}); err == nil {
		t.Fatal("notify owner must be refused")
	}
}

func TestNotice(t *testing.T) {
	k := newKind(t, map[string]any{"max_shares_per_mail": 2}, nil, nil)
	item := func(d data) reminders.Item {
		d.ExpiresAt = day(16 + d.DaysLeft)
		b, _ := json.Marshal(d)
		return reminders.Item{Subject: d.Path, Data: b}
	}
	items := slices.Values([]reminders.Item{
		item(data{Path: "/eos/user/m/marie/a.txt", File: true, Grantee: "physics", GranteeGroup: true, DaysLeft: 1}),
		item(data{Path: "/eos/user/e/einstein/b", Sharer: "einstein", Received: true, DaysLeft: 3}),
		item(data{Path: "/eos/user/m/marie/c", Grantee: "bob", DaysLeft: 6}),
	})
	got, err := k.Notice(context.Background(), reminders.Recipient{Name: "marie"}, items)
	if err != nil {
		t.Fatal(err)
	}
	shared, received := got["Shared"].([]map[string]any), got["Received"].([]map[string]any)
	if got["Username"] != "marie" || got["SoonestText"] != "tomorrow" || got["ShareCount"] != 3 || got["Remaining"] != 1 || len(shared) != 1 || len(received) != 1 {
		t.Fatalf("data %+v", got)
	}
	if shared[0]["WebURL"] != "https://cernbox.cern.ch/files/spaces/eos/user/m/marie?scrollTo=a.txt" || shared[0]["Grantee"] != "physics" || shared[0]["GranteeIsGroup"] != true {
		t.Fatalf("shared %+v", shared[0])
	}
	if received[0]["WebURL"] != "https://cernbox.cern.ch/files/shares/with-me" || received[0]["Sharer"] != "einstein" || received[0]["Name"] != "b" {
		t.Fatalf("received %+v", received[0])
	}
}
