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
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// reconcilableFS records the writes it gets. The rest of storage.FS is never
// called by the invocations.
type reconcilableFS struct {
	storage.FS
	nodes   int
	added   []string
	removed []string
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
	if got := s.reconcileInvocations().Invocations(); len(got) != 3 {
		t.Errorf("got %d invocations, want 3", len(got))
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
