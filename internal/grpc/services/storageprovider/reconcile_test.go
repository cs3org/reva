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

package storageprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// reconcilableFS records the writes it gets. The rest of storage.FS is never
// called by the invocations.
type reconcilableFS struct {
	storage.FS
	nodes   int
	added   []string
	removed []string
	// roots are the space roots ListSpaceRoots gives, and listedAt is what it
	// was called with.
	roots    []string
	listedAt string
}

func (f *reconcilableFS) ListSpaceRoots(ctx context.Context, root string, depth int) iter.Seq2[string, error] {
	f.listedAt = fmt.Sprintf("%s %d", root, depth)
	return func(yield func(string, error) bool) {
		for _, r := range f.roots {
			if !yield(r, nil) {
				return
			}
		}
	}
}

func (f *reconcilableFS) DumpNamespace(ctx context.Context, p string) iter.Seq2[reconciliation.NSDumpInfo, error] {
	return func(yield func(reconciliation.NSDumpInfo, error) bool) {
		for i := range f.nodes {
			if !yield(reconciliation.NSDumpInfo{Path: fmt.Sprintf("%s/f%d", p, i)}, nil) {
				return
			}
		}
	}
}

func (f *reconcilableFS) AddNodeACL(ctx context.Context, p string, e reconciliation.ACLEntry) error {
	f.added = append(f.added, fmt.Sprintf("%s %s:%s", p, e.Qualifier, e.Permissions))
	return nil
}

func (f *reconcilableFS) RemoveNodeACL(ctx context.Context, p string, e reconciliation.ACLEntry) error {
	f.removed = append(f.removed, fmt.Sprintf("%s %s", p, e.Qualifier))
	return nil
}

// A driver the job cannot reconcile exposes nothing, so the job can tell it
// from a provider that is down.
func TestReconcileInvocationsNeedAReconcilableDriver(t *testing.T) {
	s := &service{}
	if got := s.reconcileInvocations().Invocations(); len(got) != 0 {
		t.Errorf("got %d invocations, want none", len(got))
	}

	s = &service{storage: &reconcilableFS{}}
	if got := s.reconcileInvocations().Invocations(); len(got) != 4 {
		t.Errorf("got %d invocations, want 4", len(got))
	}
}

// A provider gives the ids of its spaces, with the space_depth of the
// production config. The roots are two levels below the mount path.
func TestListSpacesInvocationGivesSpaceIDs(t *testing.T) {
	tests := []struct {
		name       string
		mountPath  string
		spaceDepth int
		roots      []string
		wantListed string
		want       []string
	}{
		{
			name:       "a home provider",
			mountPath:  "/eos/user",
			spaceDepth: 4,
			roots:      []string{"/eos/user/n/name", "/eos/user/j/jdoe"},
			wantListed: "/eos/user 2",
			want:       []string{"/eos/user/n/name", "/eos/user/j/jdoe"},
		},
		{
			name:       "a project provider",
			mountPath:  "/eos/project",
			spaceDepth: 4,
			roots:      []string{"/eos/project/n/name"},
			wantListed: "/eos/project 2",
			want:       []string{"/eos/project/n/name"},
		},
		{
			name:       "a provider whose mount is one space",
			mountPath:  "/eos/atlas",
			spaceDepth: 2,
			want:       []string{"/eos/atlas"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &reconcilableFS{roots: tt.roots}
			s := &service{storage: fs, conf: &config{SpaceDepth: tt.spaceDepth}, mountPath: tt.mountPath}

			var got []string
			emit := func(r invoke.Result) error {
				data, err := json.Marshal(r)
				if err != nil {
					return err
				}
				var batch struct {
					Spaces []string `json:"spaces"`
				}
				if err := json.Unmarshal(data, &batch); err != nil {
					return err
				}
				got = append(got, batch.Spaces...)
				return nil
			}
			if err := s.reconcileInvocations().InvokeStream(context.Background(), reconciliation.InvocationListSpaces, map[string]any{}, emit); err != nil {
				t.Fatalf("InvokeStream: %v", err)
			}

			if fs.listedAt != tt.wantListed {
				t.Errorf("the driver listed %q, want %q", fs.listedAt, tt.wantListed)
			}
			want := make([]string, 0, len(tt.want))
			for _, p := range tt.want {
				want = append(want, spaces.EncodeSpaceID(p))
			}
			if !slices.Equal(got, want) {
				t.Errorf("space ids = %v, want the ids of %v", got, tt.want)
			}
		})
	}
}

// The dump comes in batches, and every node arrives exactly once.
func TestDumpInvocationBatchesTheNodes(t *testing.T) {
	fs := &reconcilableFS{nodes: 2*reconciliation.DumpBatchSize + 1}
	s := &service{storage: fs}
	set := s.reconcileInvocations()

	var batches []int
	seen := map[string]bool{}
	emit := func(r invoke.Result) error {
		// the result crosses the control channel as JSON
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		var batch struct {
			Nodes []reconciliation.NSDumpInfo `json:"nodes"`
		}
		if err := json.Unmarshal(data, &batch); err != nil {
			return err
		}
		batches = append(batches, len(batch.Nodes))
		for _, n := range batch.Nodes {
			seen[n.Path] = true
		}
		return nil
	}

	if err := set.InvokeStream(context.Background(), reconciliation.InvocationDumpNamespace, map[string]any{reconciliation.ArgPath: "/eos/a"}, emit); err != nil {
		t.Fatalf("InvokeStream: %v", err)
	}
	if want := []int{reconciliation.DumpBatchSize, reconciliation.DumpBatchSize, 1}; fmt.Sprint(batches) != fmt.Sprint(want) {
		t.Errorf("batches = %v, want %v", batches, want)
	}
	if len(seen) != fs.nodes {
		t.Errorf("got %d distinct nodes, want %d", len(seen), fs.nodes)
	}
}

func TestACLInvocationsDecodeTheEntry(t *testing.T) {
	fs := &reconcilableFS{}
	s := &service{storage: fs}
	set := s.reconcileInvocations()
	ctx := context.Background()

	entry, err := json.Marshal(reconciliation.ACLEntry{
		Type:        provider.GranteeType_GRANTEE_TYPE_USER,
		Qualifier:   "10001",
		Permissions: sharehierarchy.PermRW,
	})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{reconciliation.ArgPath: "/eos/a", reconciliation.ArgEntry: string(entry)}

	if _, err := set.Invoke(ctx, reconciliation.InvocationAddNodeACL, args); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := set.Invoke(ctx, reconciliation.InvocationRemoveNodeACL, args); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if fmt.Sprint(fs.added) != "[/eos/a 10001:RW]" || fmt.Sprint(fs.removed) != "[/eos/a 10001]" {
		t.Errorf("added %v and removed %v", fs.added, fs.removed)
	}

	bad := map[string]any{reconciliation.ArgPath: "/eos/a", reconciliation.ArgEntry: "not json"}
	if _, err := set.Invoke(ctx, reconciliation.InvocationAddNodeACL, bad); err == nil {
		t.Error("add with an entry that is not JSON: no error")
	}
	if len(fs.added) != 1 {
		t.Errorf("an entry that is not JSON reached the driver")
	}
}
