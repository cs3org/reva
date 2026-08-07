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
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"google.golang.org/grpc"
)

// fakeStore is an in-memory ShareStore recording which shares were marked and
// which were removed.
type fakeStore struct {
	shares     []model.Share
	marked     []string
	unshared   []string
	listErr    error
	markErr    error
	unshareErr error
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

func (f *fakeStore) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	if f.unshareErr != nil {
		return f.unshareErr
	}
	f.unshared = append(f.unshared, ref.GetId().GetOpaqueId())
	return nil
}

// fakeLinkStore is an in-memory PublicLinkStore recording which links were
// marked.
type fakeLinkStore struct {
	links   []model.PublicLink
	marked  []string
	listErr error
	markErr error
}

func (f *fakeLinkStore) ListPublicLinks(u *userpb.User, filters []*link.ListPublicSharesRequest_Filter, expiry *sql.ExpiryRange, hideOrphans bool) ([]model.PublicLink, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if !hideOrphans {
		return f.links, nil
	}
	var out []model.PublicLink
	for _, l := range f.links {
		if !l.Orphan {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeLinkStore) MarkAsOrphaned(ctx context.Context, ref *link.PublicShareReference) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.marked = append(f.marked, ref.GetId().GetOpaqueId())
	return nil
}

// fakeGateway is a gateway client driven by presence sets. Only the methods the
// jobs call are implemented; the embedded interface makes any other call panic,
// which keeps the fake honest.
type fakeGateway struct {
	gateway.GatewayAPIClient
	resources map[string]bool
	users     map[string]bool
	// userTypes overrides the type of a resolved user. Unlisted users are
	// primary accounts.
	userTypes map[string]userpb.UserType
	groups    map[string]bool
	// paths maps "<storage>/<inode>" to the path of the resource.
	paths    map[string]string
	statErr  error
	userErr  error
	groupErr error
	pathErr  error
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
	name := in.GetValue()
	if !f.users[name] {
		return &userpb.GetUserByClaimResponse{Status: status(false)}, nil
	}
	t, ok := f.userTypes[name]
	if !ok {
		t = userpb.UserType_USER_TYPE_PRIMARY
	}
	return &userpb.GetUserByClaimResponse{
		Status: status(true),
		User:   &userpb.User{Id: &userpb.UserId{OpaqueId: name, Type: t}},
	}, nil
}

func (f *fakeGateway) GetPath(ctx context.Context, in *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	if f.pathErr != nil {
		return nil, f.pathErr
	}
	id := in.GetResourceId()
	p, ok := f.paths[id.GetStorageId()+"/"+id.GetOpaqueId()]
	if !ok {
		return &provider.GetPathResponse{Status: status(false)}, nil
	}
	return &provider.GetPathResponse{Status: status(true), Path: p}, nil
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

// publicLink builds a model.PublicLink with the fields the orphan job reads.
func publicLink(id uint, instance, inode string, orphan bool) model.PublicLink {
	var l model.PublicLink
	l.Id = id
	l.Orphan = orphan
	l.Instance = instance
	l.Inode = inode
	return l
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
	if got := report.Orphaned[0]; got.ID != "6" || got.Reason != ReasonResourceMissing {
		t.Errorf("would-orphan = %+v, want share 6 resource-missing", got)
	}
	if len(store.marked) != 0 {
		t.Errorf("dry_run marked %v, want none", store.marked)
	}
}

// TestOrphanDryRunMatchesLiveRun asserts that dry_run reports exactly the shares
// a live run marks, so a dry run can be trusted as a preview.
func TestOrphanDryRunMatchesLiveRun(t *testing.T) {
	shares := []model.Share{
		share(20, "eosuser", "inode-20", "jdoe", false, false),            // valid
		share(21, "eosuser", "inode-gone", "jdoe", false, false),          // resource gone
		share(22, "eosproject", "inode-22", "defunct-group", true, false), // recipient gone
	}
	newGateway := func() *fakeGateway {
		return &fakeGateway{
			resources: map[string]bool{"eosuser/inode-20": true, "eosproject/inode-22": true},
			users:     map[string]bool{"jdoe": true},
			groups:    map[string]bool{},
		}
	}

	dryStore := &fakeStore{shares: append([]model.Share(nil), shares...)}
	dry, err := (&OrphanJob{Shares: dryStore, Gateway: newGateway(), DryRun: true}).Run(context.Background())
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	liveStore := &fakeStore{shares: append([]model.Share(nil), shares...)}
	live, err := (&OrphanJob{Shares: liveStore, Gateway: newGateway()}).Run(context.Background())
	if err != nil {
		t.Fatalf("live run: %v", err)
	}

	if len(dry.Orphaned) != len(live.Orphaned) {
		t.Fatalf("dry run reported %d orphans, live run %d", len(dry.Orphaned), len(live.Orphaned))
	}
	for i := range dry.Orphaned {
		if dry.Orphaned[i].ID != live.Orphaned[i].ID || dry.Orphaned[i].Reason != live.Orphaned[i].Reason {
			t.Errorf("orphan[%d]: dry = %+v, live = %+v", i, dry.Orphaned[i], live.Orphaned[i])
		}
	}
	if len(dryStore.marked) != 0 {
		t.Errorf("dry_run marked %v, want none", dryStore.marked)
	}
	if got, want := sortedMarked(liveStore), []string{"21", "22"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("live run marked %v, want %v", got, want)
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

// TestMarkAddressesTheRightStore asserts that an entry is marked in the store it
// came from, by the numeric id rendered as the CS3 opaque id.
func TestMarkAddressesTheRightStore(t *testing.T) {
	shares, links := &fakeStore{}, &fakeLinkStore{}
	job := &OrphanJob{Shares: shares, Links: links, Gateway: &fakeGateway{}}

	if err := job.mark(context.Background(), entry{kind: KindShare, id: "42"}); err != nil {
		t.Fatalf("mark share: %v", err)
	}
	if err := job.mark(context.Background(), entry{kind: KindPublicLink, id: "43"}); err != nil {
		t.Fatalf("mark link: %v", err)
	}

	if len(shares.marked) != 1 || shares.marked[0] != "42" {
		t.Errorf("share store marked %v, want [42]", shares.marked)
	}
	if len(links.marked) != 1 || links.marked[0] != "43" {
		t.Errorf("link store marked %v, want [43]", links.marked)
	}
}

func TestOrphanPublicLinkResourceMissing(t *testing.T) {
	links := &fakeLinkStore{links: []model.PublicLink{
		publicLink(30, "eosuser", "inode-30", false),
	}}
	gw := &fakeGateway{resources: map[string]bool{}} // resource gone
	job := &OrphanJob{Links: links, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Orphaned) != 1 {
		t.Fatalf("report = %+v, want 1 orphaned", report)
	}
	if got := report.Orphaned[0]; got.Kind != KindPublicLink || got.ID != "30" || got.Reason != ReasonResourceMissing {
		t.Errorf("orphaned = %+v, want publiclink 30 resource-missing", got)
	}
	if len(links.marked) != 1 || links.marked[0] != "30" {
		t.Errorf("marked = %v, want [30]", links.marked)
	}
}

// TestOrphanPublicLinkNeedsNoRecipient asserts that a link whose resource exists
// survives even though it has no grantee to resolve. Were the recipient check
// applied to links, the empty ShareWith would look like a missing user and
// orphan every link in the database.
func TestOrphanPublicLinkNeedsNoRecipient(t *testing.T) {
	links := &fakeLinkStore{links: []model.PublicLink{
		publicLink(31, "eosuser", "inode-31", false),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-31": true},
		users:     map[string]bool{}, // no user resolves, and none must be looked up
	}
	job := &OrphanJob{Links: links, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 1 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 1 checked / 0 orphaned", report)
	}
	if len(links.marked) != 0 {
		t.Errorf("marked = %v, want none", links.marked)
	}
}

func TestOrphanAlreadyOrphanLinkExcluded(t *testing.T) {
	links := &fakeLinkStore{links: []model.PublicLink{
		publicLink(32, "eosuser", "inode-32", true), // already orphan, resource also gone
	}}
	job := &OrphanJob{Links: links, Gateway: &fakeGateway{resources: map[string]bool{}}}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 0 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 0 checked (already-orphan filtered out)", report)
	}
}

// TestOrphanScansBothKinds asserts that one run covers both stores and reports
// each item under its own kind.
func TestOrphanScansBothKinds(t *testing.T) {
	shares := &fakeStore{shares: []model.Share{
		share(40, "eosuser", "inode-40", "jdoe", false, false),   // valid
		share(41, "eosuser", "inode-gone", "jdoe", false, false), // resource gone
	}}
	links := &fakeLinkStore{links: []model.PublicLink{
		publicLink(50, "eosuser", "inode-50", false),   // valid
		publicLink(51, "eosuser", "inode-gone", false), // resource gone
	}}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-40": true, "eosuser/inode-50": true},
		users:     map[string]bool{"jdoe": true},
	}
	job := &OrphanJob{Shares: shares, Links: links, Gateway: gw}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 4 || len(report.Orphaned) != 2 {
		t.Fatalf("report = %+v, want 4 checked / 2 orphaned", report)
	}
	byKind := map[Kind]string{}
	for _, o := range report.Orphaned {
		byKind[o.Kind] = o.ID
	}
	if byKind[KindShare] != "41" || byKind[KindPublicLink] != "51" {
		t.Errorf("orphaned = %+v, want share 41 and publiclink 51", report.Orphaned)
	}
	if len(shares.marked) != 1 || shares.marked[0] != "41" {
		t.Errorf("share store marked %v, want [41]", shares.marked)
	}
	if len(links.marked) != 1 || links.marked[0] != "51" {
		t.Errorf("link store marked %v, want [51]", links.marked)
	}
}

func TestOrphanLinkListErrorFails(t *testing.T) {
	job := &OrphanJob{
		Links:   &fakeLinkStore{listErr: errors.New("db down")},
		Gateway: &fakeGateway{},
	}

	if _, err := job.Run(context.Background()); err == nil {
		t.Fatal("Run must fail when public links cannot be listed")
	}
}

// TestOrphanMarkErrorIsNotReportedAsOrphaned asserts that a failed write is
// counted as a failure and kept out of the orphaned list, so the log and the
// report never claim a change that did not happen.
func TestOrphanMarkErrorIsNotReportedAsOrphaned(t *testing.T) {
	store := &fakeStore{
		shares:  []model.Share{share(60, "eosuser", "inode-gone", "jdoe", false, false)},
		markErr: errors.New("db write failed"),
	}
	job := &OrphanJob{Shares: store, Gateway: &fakeGateway{resources: map[string]bool{}}}

	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail on a per-item write error: %v", err)
	}
	if report.Failed != 1 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 1 failed / 0 orphaned", report)
	}
}
