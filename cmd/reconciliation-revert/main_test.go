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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

const journal = `{"level":"info","run":"r1","event":"reconciliation.shallow.start","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"1","action":"add","path":"/a","storage_id":"s","opaque_id":"10","grantee":"alice","grantee_type":"user","observed":"","expected":"RW","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"2","action":"add","path":"/a","storage_id":"s","opaque_id":"10","grantee":"alice","grantee_type":"user","observed":"","expected":"RW","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"3","action":"update","path":"/b","storage_id":"s","opaque_id":"11","grantee":"g","grantee_type":"group","observed":"R","expected":"RW","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"4","action":"update","path":"/c","storage_id":"s","opaque_id":"12","grantee":"bob","grantee_type":"user","observed":"D","expected":"R","dry_run":false}
{"level":"R","run":"r1","event":"reconciliation.shallow.remove","share":"5","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"6","action":"add","path":"/d","storage_id":"s","opaque_id":"13","grantee":"bob","grantee_type":"user","observed":"","expected":"R","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"7","action":"update","path":"/e","storage_id":"s","opaque_id":"14","grantee":"bob","grantee_type":"user","observed":"RW","expected":"R","dry_run":false}
{"level":"info","run":"r1","event":"reconciliation.shallow.grant","share":"8","action":"add","path":"/f","storage_id":"s","opaque_id":"15","grantee":"bob","grantee_type":"user","observed":"","expected":"R","dry_run":false}
`

func TestReadJournal(t *testing.T) {
	grants, run, err := ReadJournal(strings.NewReader(journal), "")
	if err != nil {
		t.Fatal(err)
	}
	if run != "r1" {
		t.Errorf("run = %q, want r1", run)
	}
	var shares []string
	for _, g := range grants {
		shares = append(shares, g.Share)
	}
	// share 2 writes the same entry as share 1 and is dropped.
	if got, want := strings.Join(shares, ","), "1,3,4,6,7,8"; got != want {
		t.Errorf("shares = %s, want %s", got, want)
	}
}

func TestReadJournalManyRuns(t *testing.T) {
	two := journal + strings.ReplaceAll(journal, `"r1"`, `"r2"`)
	if _, _, err := ReadJournal(strings.NewReader(two), ""); err == nil {
		t.Fatal("want an error for two runs without -run")
	}
	grants, run, err := ReadJournal(strings.NewReader(two), "r2")
	if err != nil {
		t.Fatal(err)
	}
	if run != "r2" || len(grants) != 6 {
		t.Errorf("got run %q with %d grants, want r2 with 6", run, len(grants))
	}
}

func TestReadJournalSkipsDryRun(t *testing.T) {
	dry := strings.ReplaceAll(journal, `"dry_run":false`, `"dry_run":true`)
	if _, _, err := ReadJournal(strings.NewReader(dry), ""); err == nil {
		t.Fatal("want an error for a journal with only dry-run lines")
	}
}

// fakeStore keeps one level per resource and recipient.
type fakeStore struct {
	entries map[string]sharehierarchy.PermLevel
	calls   []string
}

func entryKey(id, sharee string) string { return id + "/" + sharee }

func shareeOf(g *provider.Grantee) string {
	s, _ := sharehierarchy.ShareeInfo(g)
	return s
}

func ok() *rpc.Status { return &rpc.Status{Code: rpc.Code_CODE_OK} }

func (f *fakeStore) ListGrants(_ context.Context, in *provider.ListGrantsRequest, _ ...grpc.CallOption) (*provider.ListGrantsResponse, error) {
	id := in.GetRef().GetResourceId().GetOpaqueId()
	var grants []*provider.Grant
	for k, l := range f.entries {
		resource, sharee, _ := strings.Cut(k, "/")
		if resource != id {
			continue
		}
		g := &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: sharee}}}
		if sharee == "g" {
			g = &provider.Grantee{Id: &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: sharee}}}
		}
		grants = append(grants, &provider.Grant{Grantee: g, Permissions: levelPermissions(l)})
	}
	return &provider.ListGrantsResponse{Status: ok(), Grants: grants}, nil
}

func (f *fakeStore) RemoveGrant(_ context.Context, in *provider.RemoveGrantRequest, _ ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	k := entryKey(in.GetRef().GetResourceId().GetOpaqueId(), shareeOf(in.GetGrant().GetGrantee()))
	f.calls = append(f.calls, "remove "+k)
	delete(f.entries, k)
	return &provider.RemoveGrantResponse{Status: ok()}, nil
}

func (f *fakeStore) UpdateGrant(_ context.Context, in *provider.UpdateGrantRequest, _ ...grpc.CallOption) (*provider.UpdateGrantResponse, error) {
	k := entryKey(in.GetRef().GetResourceId().GetOpaqueId(), shareeOf(in.GetGrant().GetGrantee()))
	l := sharehierarchy.PermLevelFromCS3(in.GetGrant().GetPermissions())
	f.calls = append(f.calls, "update "+k+" "+l.String())
	f.entries[k] = l
	return &provider.UpdateGrantResponse{Status: ok()}, nil
}

func (f *fakeStore) DenyGrant(_ context.Context, in *provider.DenyGrantRequest, _ ...grpc.CallOption) (*provider.DenyGrantResponse, error) {
	k := entryKey(in.GetRef().GetResourceId().GetOpaqueId(), shareeOf(in.GetGrantee()))
	f.calls = append(f.calls, "deny "+k)
	f.entries[k] = sharehierarchy.PermDeny
	return &provider.DenyGrantResponse{Status: ok()}, nil
}

func newReverter(store *fakeStore, apply bool, out *bytes.Buffer) *Reverter {
	log := zerolog.New(out)
	return &Reverter{
		Grants:  func(context.Context, string) (GrantAPI, error) { return store, nil },
		Grantee: testGrantee,
		Log:     &log,
		Apply:   apply,
	}
}

// testGrantee builds a user grantee without a gateway.
func testGrantee(ctx context.Context, t, name string) (*provider.Grantee, error) {
	if t == sharehierarchy.ShareeTypeGroup {
		return (&grantees{}).resolve(ctx, t, name)
	}
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: name}},
	}, nil
}

// state after the run: 13 was removed by hand since, 14 was changed by hand
// since, 15 was already put back.
func afterRun() *fakeStore {
	return &fakeStore{entries: map[string]sharehierarchy.PermLevel{
		"10/alice": sharehierarchy.PermRW,
		"11/g":     sharehierarchy.PermRW,
		"12/bob":   sharehierarchy.PermRead,
		"14/bob":   sharehierarchy.PermDeny,
	}}
}

func events(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestRevert(t *testing.T) {
	grants, _, err := ReadJournal(strings.NewReader(journal), "")
	if err != nil {
		t.Fatal(err)
	}
	store := afterRun()
	var out bytes.Buffer
	rep, err := newReverter(store, true, &out).Revert(context.Background(), grants)
	if err != nil {
		t.Fatal(err)
	}
	if rep != (Report{Reverted: 3, Skipped: 3}) {
		t.Errorf("report = %+v", rep)
	}

	want := map[string]sharehierarchy.PermLevel{
		"11/g":   sharehierarchy.PermRead,
		"12/bob": sharehierarchy.PermDeny,
		"14/bob": sharehierarchy.PermDeny,
	}
	if len(store.entries) != len(want) {
		t.Errorf("entries = %v, want %v", store.entries, want)
	}
	for k, l := range want {
		if store.entries[k] != l {
			t.Errorf("entry %s = %s, want %s", k, store.entries[k], l)
		}
	}
	if got, want := strings.Join(store.calls, "; "), "remove 10/alice; update 11/g R; deny 12/bob"; got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}

	reasons := map[string]string{}
	for _, e := range events(t, &out) {
		if e["event"] == EventSkip {
			reasons[e["share"].(string)] = e["reason"].(string)
		}
		if e["event"] == EventRevert && e["share"] == "3" && (e["before"] != "RW" || e["written"] != "R" || e["op"] != OpUpdate) {
			t.Errorf("revert line of share 3 = %v", e)
		}
	}
	if reasons["6"] != ReasonReverted || reasons["7"] != ReasonDrifted || reasons["8"] != ReasonReverted {
		t.Errorf("skip reasons = %v", reasons)
	}
}

func TestRevertDryRun(t *testing.T) {
	grants, _, err := ReadJournal(strings.NewReader(journal), "")
	if err != nil {
		t.Fatal(err)
	}
	store := afterRun()
	var out bytes.Buffer
	rep, err := newReverter(store, false, &out).Revert(context.Background(), grants)
	if err != nil {
		t.Fatal(err)
	}
	if rep != (Report{Reverted: 3, Skipped: 3}) {
		t.Errorf("report = %+v", rep)
	}
	if len(store.calls) != 0 {
		t.Errorf("dry run wrote: %v", store.calls)
	}
}
