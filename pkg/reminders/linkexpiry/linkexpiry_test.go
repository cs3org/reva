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

package linkexpiry

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

// fakeAdmin answers the expiring invocation two links at a time, and the
// space invocation from spaces.
type fakeAdmin struct {
	adminpb.AdminAPIClient
	links  []link
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
		start = slices.IndexFunc(f.links, func(l link) bool { return l.ID == uint(n) }) + 1
	}
	page := f.links[start:min(start+2, len(f.links))]
	b, _ := json.Marshal(map[string]any{"links": page})
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

func newKind(t *testing.T, links []link, paths map[string]string) *kind {
	t.Helper()
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "linkexpiry-test"})
	if sharedconf.GetJWTSecret("") == "" {
		t.Skip("shared config initialised without a secret")
	}
	k, err := New(context.Background(), map[string]any{"max_links_per_mail": 1})
	if err != nil {
		t.Fatal(err)
	}
	kk := k.(*kind)
	kk.admin = &fakeAdmin{links: links, spaces: map[string]any{
		"project-c": map[string]any{"name": "cernbox", "roles": []map[string]any{
			{"name": "reader", "groups": []string{"cernbox-readers"}},
			{"name": "admin", "users": []string{"alice"}, "groups": []string{"cernbox-admins"}},
		}},
		"project-n": map[string]any{"name": "noadmins", "roles": []map[string]any{}},
	}}
	kk.gateway = &fakeGateway{paths: paths}
	kk.now = func() time.Time { return time.Date(2026, 10, 16, 8, 0, 0, 0, time.Local) }
	return kk
}

func day(d int) time.Time { return time.Date(2026, 10, d, 10, 0, 0, 0, time.Local) }

func TestScan(t *testing.T) {
	old := day(1)
	k := newKind(t, []link{
		{ID: 1, Token: "t1", Initiator: "einstein", Inode: "1", ExpiresAt: day(17), UpdatedAt: old},
		{ID: 2, Token: "t2", Initiator: "einstein", Inode: "2", ExpiresAt: day(23), UpdatedAt: old},
		{ID: 3, Token: "t3", Initiator: "marie", Inode: "3", ExpiresAt: day(20), UpdatedAt: old},
		// created with 3 days left: the 7-day point is skipped
		{ID: 4, Token: "t4", Initiator: "marie", Inode: "4", ExpiresAt: day(19), UpdatedAt: day(16)},
		{ID: 5, Token: "t5", Initiator: "marie", Inode: "5", ExpiresAt: day(19), UpdatedAt: old},
		{ID: 6, Token: "t6", Initiator: "marie", Inode: "6", ExpiresAt: day(19), UpdatedAt: old},
		{ID: 7, Token: "t7", Initiator: "marie", Inode: "7", ExpiresAt: day(19), UpdatedAt: old},
	}, map[string]string{
		"1": "/eos/user/e/einstein/a",
		"2": "/eos/user/e/einstein/b",
		"3": "/eos/project/c/cernbox/docs",
		"4": "/eos/user/m/marie/d",
		"6": "error",
		"7": "/eos/project/n/noadmins/e",
	})

	type got struct {
		subject string
		level   int
		to      reminders.Recipient
	}
	var items []got
	var errs int
	for it, err := range k.Scan(context.Background()) {
		if err != nil {
			errs++
			continue
		}
		items = append(items, got{it.Subject, it.Level, it.Recipient})
	}
	want := []got{
		{"link:2026-10-17:1:einstein", 2, reminders.Recipient{Name: "einstein"}},
		{"link:2026-10-23:2:einstein", 1, reminders.Recipient{Name: "einstein"}},
		{"link:2026-10-20:3:alice", 1, reminders.Recipient{Name: "alice"}},
		{"link:2026-10-20:3:cernbox-admins", 1, reminders.Recipient{Name: "cernbox-admins", Group: true}},
		{"link:2026-10-19:4:marie", 0, reminders.Recipient{Name: "marie"}},
		// a project without the role: the initiator
		{"link:2026-10-19:7:marie", 1, reminders.Recipient{Name: "marie"}},
	}
	if !slices.Equal(items, want) || errs != 1 {
		t.Fatalf("got %+v and %d errors, want %+v and 1 error", items, errs, want)
	}
}

func TestNotice(t *testing.T) {
	k := newKind(t, nil, nil)
	item := func(token, p string, daysLeft int) reminders.Item {
		d, _ := json.Marshal(data{Token: token, Path: p, File: true, Project: "cernbox", ExpiresAt: day(16 + daysLeft), DaysLeft: daysLeft})
		return reminders.Item{Subject: token, Data: d}
	}
	items := slices.Values([]reminders.Item{item("t1", "/eos/project/c/cernbox/a.txt", 1), item("t2", "/eos/project/c/cernbox/b.txt", 6)})
	got, err := k.Notice(context.Background(), reminders.Recipient{Name: "cernbox-admins", Group: true}, items)
	if err != nil {
		t.Fatal(err)
	}
	links := got["Links"].([]map[string]any)
	if got["Username"] != "cernbox" || got["IsProject"] != true || got["SoonestText"] != "tomorrow" || got["LinkCount"] != 2 || got["Remaining"] != 1 || len(links) != 1 {
		t.Fatalf("data %+v", got)
	}
	if links[0]["Href"] != "https://cernbox.cern.ch/s/t1" || links[0]["WebURL"] != "https://cernbox.cern.ch/files/spaces/eos/project/c/cernbox?scrollTo=a.txt" || links[0]["Name"] != "Link" {
		t.Fatalf("link %+v", links[0])
	}
}
