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

package reconciliation

import (
	"context"
	"errors"
	"sort"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"google.golang.org/grpc"
)

// fakeStore is an in-memory ShareStore recording which shares were marked.
type fakeStore struct {
	shares  []model.Share
	marked  []string
	listErr error
	markErr error
}

func (f *fakeStore) ListModelShares(u *userpb.User, filters []*collaboration.Filter, hideOrphans bool) ([]model.Share, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if !hideOrphans {
		return f.shares, nil
	}
	var out []model.Share
	for _, s := range f.shares {
		if !s.Orphan {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.marked = append(f.marked, ref.GetId().GetOpaqueId())
	return nil
}

// fakeGateway is a gateway client driven by presence sets. Only the three
// methods the orphan job calls are implemented; the embedded interface makes any
// other call panic, which keeps the fake honest.
type fakeGateway struct {
	gateway.GatewayAPIClient
	resources map[string]bool
	users     map[string]bool
	groups    map[string]bool
	statErr   error
	userErr   error
	groupErr  error
}

func status(present bool) *rpc.Status {
	if present {
		return &rpc.Status{Code: rpc.Code_CODE_OK}
	}
	return &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}
}

func (f *fakeGateway) Stat(ctx context.Context, in *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	id := in.GetRef().GetResourceId()
	return &provider.StatResponse{Status: status(f.resources[id.StorageId+"/"+id.OpaqueId])}, nil
}

func (f *fakeGateway) GetUserByClaim(ctx context.Context, in *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if f.userErr != nil {
		return nil, f.userErr
	}
	return &userpb.GetUserByClaimResponse{Status: status(f.users[in.GetValue()])}, nil
}

func (f *fakeGateway) GetGroupByClaim(ctx context.Context, in *grouppb.GetGroupByClaimRequest, _ ...grpc.CallOption) (*grouppb.GetGroupByClaimResponse, error) {
	if f.groupErr != nil {
		return nil, f.groupErr
	}
	return &grouppb.GetGroupByClaimResponse{Status: status(f.groups[in.GetValue()])}, nil
}

// share builds a model.Share with the fields the orphan job reads.
func share(id uint, instance, inode, shareWith string, isGroup, orphan bool) model.Share {
	var s model.Share
	s.Id = id
	s.Orphan = orphan
	s.Instance = instance
	s.Inode = inode
	s.ShareWith = shareWith
	s.SharedWithIsGroup = isGroup
	return s
}

func sortedMarked(f *fakeStore) []string {
	out := append([]string(nil), f.marked...)
	sort.Strings(out)
	return out
}

func TestOrphanResourceMissing(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(1, "eosuser", "inode-1", "jdoe", false, false),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{}, // resource gone
		users:     map[string]bool{"jdoe": true},
	}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 1 || len(report.Orphaned) != 1 {
		t.Fatalf("report = %+v, want 1 checked / 1 orphaned", report)
	}
	if report.Orphaned[0].Reason != ReasonResourceMissing {
		t.Errorf("reason = %q, want %q", report.Orphaned[0].Reason, ReasonResourceMissing)
	}
	if got := sortedMarked(store); len(got) != 1 || got[0] != "1" {
		t.Errorf("marked = %v, want [1]", got)
	}
}

func TestOrphanUserRecipientMissing(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(2, "eosuser", "inode-2", "ghost", false, false),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-2": true},
		users:     map[string]bool{}, // user gone
	}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Orphaned) != 1 || report.Orphaned[0].Reason != ReasonRecipientMissing {
		t.Fatalf("report = %+v, want 1 recipient-missing", report)
	}
	if got := sortedMarked(store); len(got) != 1 || got[0] != "2" {
		t.Errorf("marked = %v, want [2]", got)
	}
}

func TestOrphanGroupRecipientMissing(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(3, "eosproject", "inode-3", "defunct-group", true, false),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosproject/inode-3": true},
		groups:    map[string]bool{},                      // group gone
		users:     map[string]bool{"defunct-group": true}, // must be ignored: it is a group
	}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Orphaned) != 1 || report.Orphaned[0].Reason != ReasonRecipientMissing {
		t.Fatalf("report = %+v, want 1 recipient-missing", report)
	}
	if got := sortedMarked(store); len(got) != 1 || got[0] != "3" {
		t.Errorf("marked = %v, want [3]", got)
	}
}

func TestOrphanAllPresentMarksNothing(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(4, "eosuser", "inode-4", "jdoe", false, false),
		share(5, "eosproject", "inode-5", "cern-users", true, false),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-4": true, "eosproject/inode-5": true},
		users:     map[string]bool{"jdoe": true},
		groups:    map[string]bool{"cern-users": true},
	}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 2 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 2 checked / 0 orphaned", report)
	}
	if len(store.marked) != 0 {
		t.Errorf("marked = %v, want none", store.marked)
	}
}

func TestOrphanDryRunMarksNothing(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(6, "eosuser", "inode-6", "jdoe", false, false),
	}}
	gw := &fakeGateway{resources: map[string]bool{}, users: map[string]bool{"jdoe": true}}
	job := &OrphanJob{Shares: store, Gateway: gw, DryRun: true}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.DryRun || len(report.Orphaned) != 1 {
		t.Fatalf("report = %+v, want dry-run with 1 would-orphan", report)
	}
	if len(store.marked) != 0 {
		t.Errorf("dry_run marked %v, want none", store.marked)
	}
}

func TestOrphanLookupErrorSkips(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(7, "eosuser", "inode-7", "jdoe", false, false),
	}}
	gw := &fakeGateway{statErr: errors.New("gateway down")}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail on a per-share lookup error: %v", err)
	}
	if report.Skipped != 1 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 1 skipped / 0 orphaned", report)
	}
	if len(store.marked) != 0 {
		t.Errorf("marked %v on lookup error, want none (no false orphan)", store.marked)
	}
}

func TestOrphanAlreadyOrphanExcluded(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(8, "eosuser", "inode-8", "jdoe", false, true), // already orphan, resource also gone
	}}
	gw := &fakeGateway{resources: map[string]bool{}, users: map[string]bool{}}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 0 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 0 checked (already-orphan filtered out)", report)
	}
	if len(store.marked) != 0 {
		t.Errorf("marked %v, want none", store.marked)
	}
}

func TestOrphanMixedBatch(t *testing.T) {
	store := &fakeStore{shares: []model.Share{
		share(10, "eosuser", "inode-10", "jdoe", false, false),           // valid
		share(11, "eosuser", "inode-11", "ghost", false, false),          // recipient gone
		share(12, "eosproject", "inode-gone", "cern-users", true, false), // resource gone
		share(13, "eosuser", "inode-13", "jdoe", false, true),            // already orphan, excluded
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-10": true, "eosuser/inode-11": true},
		users:     map[string]bool{"jdoe": true},
		groups:    map[string]bool{"cern-users": true},
	}
	job := &OrphanJob{Shares: store, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 3 || len(report.Orphaned) != 2 {
		t.Fatalf("report = %+v, want 3 checked / 2 orphaned", report)
	}
	if got, want := sortedMarked(store), []string{"11", "12"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("marked = %v, want %v", got, want)
	}
}

func TestOrphanListErrorFails(t *testing.T) {
	store := &fakeStore{listErr: errors.New("db down")}
	job := &OrphanJob{Shares: store, Gateway: &fakeGateway{}}

	if _, err := job.Run(context.Background()); err == nil {
		t.Fatal("Run must fail when shares cannot be listed")
	}
}

func TestShareRefByID(t *testing.T) {
	ref := shareRefByID(42)
	if got := ref.GetId().GetOpaqueId(); got != "42" {
		t.Errorf("opaque id = %q, want %q", got, "42")
	}
}
