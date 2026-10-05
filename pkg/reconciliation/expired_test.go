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
	"testing"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type expiredStore struct {
	*fakeStore
}

func (f expiredStore) ListExpiredShares(ctx context.Context) ([]*collaboration.Share, error) {
	all, err := f.ListShares(ctx, nil)
	var out []*collaboration.Share
	for _, s := range all {
		if isExpired(s, time.Now()) {
			out = append(out, s)
		}
	}
	return out, err
}

type expiredGrants struct {
	calls     []string
	removeErr error
}

func (f *expiredGrants) AddGrant(ctx context.Context, in *provider.AddGrantRequest, _ ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	f.calls = append(f.calls, "add "+in.GetRef().GetResourceId().GetOpaqueId()+" "+sharehierarchy.PermLevelFromCS3(in.GetGrant().GetPermissions()).String())
	return &provider.AddGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (f *expiredGrants) DenyGrant(ctx context.Context, in *provider.DenyGrantRequest, _ ...grpc.CallOption) (*provider.DenyGrantResponse, error) {
	f.calls = append(f.calls, "deny "+in.GetRef().GetResourceId().GetOpaqueId())
	return &provider.DenyGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (f *expiredGrants) RemoveGrant(ctx context.Context, in *provider.RemoveGrantRequest, _ ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	f.calls = append(f.calls, "remove "+in.GetRef().GetResourceId().GetOpaqueId())
	return &provider.RemoveGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func expiring(s storedShare, at time.Time) storedShare {
	s.share = proto.Clone(s.share).(*collaboration.Share)
	s.share.Expiration = &types.Timestamp{Seconds: uint64(at.Unix())}
	return s
}

func expiredJob(store *fakeStore, gw *fakeGateway, grants *expiredGrants, dryRun bool) *ExpiredJob {
	return &ExpiredJob{
		ShareStore: expiredStore{store},
		Gateway:    gw,
		Grants:     func(context.Context, string) (ExpiredGrantStore, error) { return grants, nil },
		DryRun:     dryRun,
	}
}

func hierarchyFixture() (*fakeStore, *fakeGateway) {
	past := time.Now().Add(-time.Hour)
	store := &fakeStore{shares: []storedShare{
		shared(1, "space-a", "top", "jdoe", false, ocsRead),
		expiring(shared(2, "space-a", "mid", "jdoe", false, ocsWrite), past),
		shared(3, "space-a", "low", "jdoe", false, ocsWrite),
		// not live, so never put back.
		expiring(shared(4, "space-a", "low2", "jdoe", false, ocsWrite), past),
		// another recipient and a not yet expired share: left alone.
		shared(5, "space-a", "low", "other", false, ocsRead),
		expiring(shared(6, "space-a", "top", "other", false, ocsWrite), time.Now().Add(time.Hour)),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "other": true},
		paths: map[string]string{
			"eosuser/top":  "/eos/p/proj",
			"eosuser/mid":  "/eos/p/proj/a",
			"eosuser/low":  "/eos/p/proj/a/b",
			"eosuser/low2": "/eos/p/proj/a/c",
		},
	}
	return store, gw
}

func TestExpiredRemovesGrantAndReappliesHierarchy(t *testing.T) {
	store, gw := hierarchyFixture()
	grants := &expiredGrants{}

	report, err := expiredJob(store, gw, grants, false).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Expired != 2 || !slices.Equal(report.Removed, []string{"2", "4"}) || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	want := []string{
		"remove mid", "add mid R", "add low RW",
		"remove low2", "add low2 R",
	}
	if !slices.Equal(grants.calls, want) {
		t.Fatalf("calls = %v, want %v", grants.calls, want)
	}
	if !slices.Equal(store.unshared, []string{"2", "4"}) {
		t.Fatalf("unshared = %v", store.unshared)
	}
}

func TestExpiredDryRunChangesNothing(t *testing.T) {
	store, gw := hierarchyFixture()
	grants := &expiredGrants{}

	report, err := expiredJob(store, gw, grants, true).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(report.Removed, []string{"2", "4"}) {
		t.Fatalf("removed = %v", report.Removed)
	}
	if len(grants.calls) != 0 || len(store.unshared) != 0 {
		t.Fatalf("dry run wrote: calls %v, unshared %v", grants.calls, store.unshared)
	}
}

func TestExpiredKeepsRowWhenRemoveFails(t *testing.T) {
	store, gw := hierarchyFixture()
	grants := &expiredGrants{removeErr: errors.New("eos down")}

	report, err := expiredJob(store, gw, grants, false).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 2 || len(report.Removed) != 0 || len(store.unshared) != 0 {
		t.Fatalf("report %+v, unshared %v", report, store.unshared)
	}
}

func TestExpiredFailsWhenRelatedPathDoesNotResolve(t *testing.T) {
	store, gw := hierarchyFixture()
	delete(gw.paths, "eosuser/top")
	grants := &expiredGrants{}

	report, err := expiredJob(store, gw, grants, false).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 2 || len(grants.calls) != 0 || len(store.unshared) != 0 {
		t.Fatalf("report %+v, calls %v, unshared %v", report, grants.calls, store.unshared)
	}
}
