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
	"slices"
	"sort"
	"strconv"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"google.golang.org/grpc"
)

// storedShare is a row of fakeStore: a CS3 share plus the orphan flag the store
// filters on, which the CS3 type cannot carry.
type storedShare struct {
	share  *collaboration.Share
	orphan bool
}

// storedLink is the same for fakeLinkStore.
type storedLink struct {
	link   *link.PublicShare
	orphan bool
}

// fakeStore is an in-memory ShareStore recording which shares were marked and
// which were removed.
type fakeStore struct {
	shares     []storedShare
	marked     []string
	unshared   []string
	listErr    error
	markErr    error
	unshareErr error
	// listHook, when set, runs before every listing with the spaces it is
	// narrowed to and the number of listings already done. It lets a test change
	// the shares under a run, the way a user sharing during a run does.
	listHook func(f *fakeStore, spaces []string, done int)
	// lists counts the listings, so a test can assert which spaces a run looked
	// at again.
	lists      int
	spaceLists int
}

func (f *fakeStore) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	// only the space filters are honoured, the ones the jobs narrow a listing
	// with. They are ORed, the way the share managers group them.
	var spaces []string
	for _, filter := range filters {
		if filter.GetType() == collaboration.Filter_TYPE_SPACE_ID {
			spaces = append(spaces, filter.GetSpaceId())
		}
	}

	if f.listHook != nil {
		f.listHook(f, spaces, f.lists)
	}
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}

	var out []*collaboration.Share
	for _, s := range f.shares {
		if s.orphan {
			continue
		}
		if len(spaces) > 0 && !slices.Contains(spaces, s.share.GetResourceId().GetSpaceId()) {
			continue
		}
		out = append(out, s.share)
	}
	return out, nil
}

// ListShareSpaces counts separately from the listings: a test asserts on both.
func (f *fakeStore) ListShareSpaces(ctx context.Context) ([]string, error) {
	f.spaceLists++
	if f.listErr != nil {
		return nil, f.listErr
	}

	var out []string
	seen := map[string]struct{}{}
	for _, s := range f.shares {
		if s.orphan {
			continue
		}
		space := s.share.GetResourceId().GetSpaceId()
		if _, ok := seen[space]; ok {
			continue
		}
		seen[space] = struct{}{}
		out = append(out, space)
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
	links   []storedLink
	marked  []string
	listErr error
	markErr error
}

// ListPublicSharesInSpaces returns every space's links when given no space, the
// way the sql driver does.
func (f *fakeLinkStore) ListPublicSharesInSpaces(ctx context.Context, spaceIDs []string) ([]*link.PublicShare, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*link.PublicShare
	for _, l := range f.links {
		if l.orphan {
			continue
		}
		if len(spaceIDs) > 0 && !slices.Contains(spaceIDs, l.link.GetResourceId().GetSpaceId()) {
			continue
		}
		out = append(out, l.link)
	}
	return out, nil
}

func (f *fakeLinkStore) ListPublicShareSpaces(ctx context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	var out []string
	for _, l := range f.links {
		if l.orphan {
			continue
		}
		if space := l.link.GetResourceId().GetSpaceId(); !slices.Contains(out, space) {
			out = append(out, space)
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
	paths map[string]string
	// userLookups counts the GetUserByClaim calls, one per name the caller did
	// not already have.
	userLookups int
	statErr     error
	userErr     error
	groupErr    error
	pathErr     error
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
	f.userLookups++
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

// share builds a share with the fields the orphan job reads.
func share(id uint, instance, inode, shareWith string, isGroup, orphan bool) storedShare {
	s := &collaboration.Share{
		Id:         &collaboration.ShareId{OpaqueId: strconv.FormatUint(uint64(id), 10)},
		ResourceId: &provider.ResourceId{StorageId: instance, OpaqueId: inode},
	}
	if isGroup {
		s.Grantee = &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: shareWith}},
		}
	} else {
		s.Grantee = &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: shareWith}},
		}
	}
	return storedShare{share: s, orphan: orphan}
}

// publicLink builds a public link with the fields the orphan job reads.
func publicLink(id uint, instance, inode string, orphan bool) storedLink {
	return storedLink{
		link: &link.PublicShare{
			Id:         &link.PublicShareId{OpaqueId: strconv.FormatUint(uint64(id), 10)},
			ResourceId: &provider.ResourceId{StorageId: instance, OpaqueId: inode},
		},
		orphan: orphan,
	}
}

func sortedMarked(f *fakeStore) []string {
	out := append([]string(nil), f.marked...)
	sort.Strings(out)
	return out
}

func TestOrphanResourceMissing(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	shares := []storedShare{
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

	dryStore := &fakeStore{shares: append([]storedShare(nil), shares...)}
	dry, err := (&OrphanJob{Shares: dryStore, Gateway: newGateway(), DryRun: true}).Run(context.Background())
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	liveStore := &fakeStore{shares: append([]storedShare(nil), shares...)}
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
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	store := &fakeStore{shares: []storedShare{
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
	links := &fakeLinkStore{links: []storedLink{
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
	links := &fakeLinkStore{links: []storedLink{
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
	links := &fakeLinkStore{links: []storedLink{
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
	shares := &fakeStore{shares: []storedShare{
		share(40, "eosuser", "inode-40", "jdoe", false, false),   // valid
		share(41, "eosuser", "inode-gone", "jdoe", false, false), // resource gone
	}}
	links := &fakeLinkStore{links: []storedLink{
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
		shares:  []storedShare{share(60, "eosuser", "inode-gone", "jdoe", false, false)},
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

// TestOrphanReadsOneSpacePerListing asserts that one space per batch reads the
// items of each space on its own, in a fixed order. Holding every item of the
// database at once is what the per-space walk avoids.
func TestOrphanReadsOneSpacePerListing(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		share(70, "eosuser", "inode-70", "jdoe", false, false),
		share(71, "eosuser", "inode-71", "jdoe", false, false),
	}}
	store.shares[0].share.ResourceId.SpaceId = "space-b"
	store.shares[1].share.ResourceId.SpaceId = "space-a"
	var listed [][]string
	store.listHook = func(_ *fakeStore, spaces []string, _ int) {
		listed = append(listed, spaces)
	}
	gw := &fakeGateway{
		resources: map[string]bool{"eosuser/inode-70": true, "eosuser/inode-71": true},
		users:     map[string]bool{"jdoe": true},
	}

	job := &OrphanJob{Shares: store, Gateway: gw, SpacesPerBatch: 1}
	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 2 || len(report.Orphaned) != 0 {
		t.Fatalf("report = %+v, want 2 checked / 0 orphaned", report)
	}
	want := [][]string{{"space-a"}, {"space-b"}}
	if !slices.EqualFunc(listed, want, slices.Equal) {
		t.Errorf("listings = %v, want %v", listed, want)
	}
}

// TestOrphanBatchesSpacesIntoOneListing asserts that SpacesPerBatch spaces are
// covered by a single listing.
func TestOrphanBatchesSpacesIntoOneListing(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		share(72, "eosuser", "inode-72", "jdoe", false, false),
		share(73, "eosuser", "inode-73", "jdoe", false, false),
	}}
	store.shares[0].share.ResourceId.SpaceId = "space-a"
	store.shares[1].share.ResourceId.SpaceId = "space-b"
	var listed [][]string
	store.listHook = func(_ *fakeStore, spaces []string, _ int) {
		listed = append(listed, spaces)
	}
	gw := &fakeGateway{
		resources: map[string]bool{}, // both resources gone
		users:     map[string]bool{"jdoe": true},
	}

	job := &OrphanJob{Shares: store, Gateway: gw, SpacesPerBatch: 2}
	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Orphaned) != 2 {
		t.Fatalf("report = %+v, want 2 orphaned", report)
	}
	want := [][]string{{"space-a", "space-b"}}
	if !slices.EqualFunc(listed, want, slices.Equal) {
		t.Errorf("listings = %v, want %v", listed, want)
	}
}

// TestOrphanVisitsSpacesOfBothStores asserts that a space holding only a public
// link is visited too. Both stores are asked which spaces they hold, since a
// link needs no share next to it.
func TestOrphanVisitsSpacesOfBothStores(t *testing.T) {
	shares := &fakeStore{shares: []storedShare{
		share(74, "eosuser", "inode-74", "jdoe", false, false),
	}}
	shares.shares[0].share.ResourceId.SpaceId = "space-a"
	links := &fakeLinkStore{links: []storedLink{
		publicLink(75, "eosuser", "inode-75", false),
	}}
	links.links[0].link.ResourceId.SpaceId = "space-b"
	gw := &fakeGateway{resources: map[string]bool{}} // both resources gone

	job := &OrphanJob{Shares: shares, Links: links, Gateway: gw}
	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 2 || len(report.Orphaned) != 2 {
		t.Fatalf("report = %+v, want 2 checked / 2 orphaned", report)
	}
	if len(shares.marked) != 1 || len(links.marked) != 1 {
		t.Errorf("marked shares %v and links %v, want one of each", shares.marked, links.marked)
	}
}

// TestOrphanStopsWhenCancelled asserts that a cancelled run gives back the
// cancellation instead of checking its way through the spaces it has left.
func TestOrphanStopsWhenCancelled(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		share(76, "eosuser", "inode-76", "jdoe", false, false),
		share(77, "eosuser", "inode-77", "jdoe", false, false),
	}}
	store.shares[0].share.ResourceId.SpaceId = "space-a"
	store.shares[1].share.ResourceId.SpaceId = "space-b"
	ctx, cancel := context.WithCancel(context.Background())
	// the run is cancelled while it is checking the first space.
	store.listHook = func(_ *fakeStore, spaces []string, _ int) {
		if slices.Contains(spaces, "space-a") {
			cancel()
		}
	}
	gw := &fakeGateway{
		resources: map[string]bool{}, // every resource gone
		users:     map[string]bool{"jdoe": true},
	}

	job := &OrphanJob{Shares: store, Gateway: gw, SpacesPerBatch: 1}
	report, err := job.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want the cancellation", err)
	}
	if report.Checked != 1 || len(store.marked) != 1 || store.marked[0] != "76" {
		t.Errorf("report = %+v / marked = %v, want only the first space checked", report, store.marked)
	}
}

// TestOrphanReadsEverythingWithoutABatchSize asserts that a run without
// SpacesPerBatch reads every item in one listing per store and never asks which
// spaces there are, the way the job worked before it could batch.
func TestOrphanReadsEverythingWithoutABatchSize(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		share(78, "eosuser", "inode-78", "jdoe", false, false),
		share(79, "eosuser", "inode-79", "jdoe", false, false),
	}}
	store.shares[0].share.ResourceId.SpaceId = "space-a"
	store.shares[1].share.ResourceId.SpaceId = "space-b"
	var listed [][]string
	store.listHook = func(_ *fakeStore, spaces []string, _ int) {
		listed = append(listed, spaces)
	}
	gw := &fakeGateway{
		resources: map[string]bool{}, // both resources gone
		users:     map[string]bool{"jdoe": true},
	}

	report, err := (&OrphanJob{Shares: store, Gateway: gw}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 2 || len(report.Orphaned) != 2 {
		t.Fatalf("report = %+v, want 2 checked / 2 orphaned", report)
	}
	if store.spaceLists != 0 {
		t.Errorf("asked for the spaces %d times, want 0 without a batch size", store.spaceLists)
	}
	want := [][]string{nil}
	if !slices.EqualFunc(listed, want, slices.Equal) {
		t.Errorf("listings = %v, want one listing with no space narrowing it", listed)
	}
}
