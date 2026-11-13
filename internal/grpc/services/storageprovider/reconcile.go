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

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/pkg/errors"
)

// This file contains invokable functions which internal services
// can call. These functions are meant to allow a job to reconcile
// a state in the database with a state on the filesystem,
// for example to reconcile the share db with ACLs on a filesystem.
//
// In order to make that possible, filesystems need to expose some
// additional methods. They need to expose a way to dump a given
// namespace (so that it can be ingested for comparison),
// and they need to expose a way to non-recursively change attributes.
//
// This file adds invokable methods for this purpose. Whether the methods
// are exposed is determined by a runtime check of whether the FS has
// the necessary methods (which are defined as a `ReconcilableFS`).

// RegistryMetadata lets the job find the provider of a storage id in the
// service registry.
func (s *service) RegistryMetadata() map[string]string {
	return map[string]string{svcregistry.MetaMountID: s.mountID}
}

func (s *service) Invocations() []invoke.InvocationSpec { return s.invocations.Invocations() }

func (s *service) Invoke(ctx context.Context, name string, args map[string]any) (invoke.Result, error) {
	return s.invocations.Invoke(ctx, name, args)
}

func (s *service) InvokeStream(ctx context.Context, name string, args map[string]any, emit invoke.StreamEmit) error {
	return s.invocations.InvokeStream(ctx, name, args, emit)
}

type dumpArgs struct {
	Path string `arg:"path,required" help:"the storage path to dump"`
}

type nodeACLArgs struct {
	Path  string `arg:"path,required" help:"the storage path of the node"`
	Entry string `arg:"entry,required" help:"the ACL entry as JSON"`
}

// reconcileInvocations is empty for a driver the deep job cannot reconcile.
func (s *service) reconcileInvocations() *invoke.Set {
	set := invoke.NewSet()
	fs, ok := s.storage.(reconciliation.ReconcilableFS)
	if !ok {
		return set
	}

	set.Add(reconciliation.InvocationDumpNamespace, "Stream the nodes under a path with their ACLs, for the deep reconciliation job.").
		HandleStream(invoke.TypedStream(func(ctx context.Context, a dumpArgs, emit invoke.StreamEmit) error {
			batch := make([]reconciliation.NSDumpInfo, 0, reconciliation.DumpBatchSize)
			for n, err := range fs.DumpNamespace(ctx, a.Path) {
				if err != nil {
					return err
				}
				batch = append(batch, n)
				if len(batch) < reconciliation.DumpBatchSize {
					continue
				}
				if err := emit(invoke.Result{"nodes": batch}); err != nil {
					return err
				}
				batch = make([]reconciliation.NSDumpInfo, 0, reconciliation.DumpBatchSize)
			}
			if len(batch) == 0 {
				return nil
			}
			return emit(invoke.Result{"nodes": batch})
		}))

	set.Add(reconciliation.InvocationAddNodeACL, "Set one ACL entry on one node, not on its children.").
		Dangerous().
		Handle(invoke.Typed(func(ctx context.Context, a nodeACLArgs) (invoke.Result, error) {
			var e reconciliation.ACLEntry
			if err := json.Unmarshal([]byte(a.Entry), &e); err != nil {
				return nil, errors.Wrap(err, "storageprovider: decoding the acl entry")
			}
			if err := fs.AddNodeACL(ctx, a.Path, e); err != nil {
				return nil, err
			}
			return invoke.Result{}, nil
		}))

	set.Add(reconciliation.InvocationRemoveNodeACL, "Remove the ACL entry of one grantee from one node, not from its children.").
		Dangerous().
		Handle(invoke.Typed(func(ctx context.Context, a nodeACLArgs) (invoke.Result, error) {
			var e reconciliation.ACLEntry
			if err := json.Unmarshal([]byte(a.Entry), &e); err != nil {
				return nil, errors.Wrap(err, "storageprovider: decoding the acl entry")
			}
			if err := fs.RemoveNodeACL(ctx, a.Path, e); err != nil {
				return nil, err
			}
			return invoke.Result{}, nil
		}))

	return set
}
