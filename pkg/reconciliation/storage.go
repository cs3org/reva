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
	"iter"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// ACLEntry is one entry of the ACL of a node. The storage maps its own format
// to the permission levels, so the job compares entries without knowing it.
type ACLEntry struct {
	Type provider.GranteeType `json:"type"`
	// Qualifier is the uid number of a user and the name of a group: the storage
	// keeps users by uid, and there are far fewer shares to resolve than nodes.
	Qualifier   string                   `json:"qualifier"`
	Permissions sharehierarchy.PermLevel `json:"permissions"`
}

// NSDumpInfo is one node of a namespace dump.
type NSDumpInfo struct {
	Path string                `json:"path"`
	ID   string                `json:"id"`
	Type provider.ResourceType `json:"type"`
	ACLs []*ACLEntry           `json:"acls"`
	// ACLsOf is the path of the node whose ACLs this node must carry, empty for
	// the node itself. A storage that keeps a copy of the ACL of a file
	// elsewhere reports that copy with the path of the file.
	ACLsOf string `json:"acls_of,omitempty"`
}

// ReconcileMethods is what the deep job needs from a storage driver. The
// grant calls of the storage provider always apply an ACL recursively, but the
// job compares and fixes each node on its own.
type ReconcileMethods interface {
	// DumpNamespace does the work when the caller ranges over the result. A
	// failure gives one error and ends the sequence.
	DumpNamespace(ctx context.Context, path string) iter.Seq2[NSDumpInfo, error]
	// AddNodeACL sets e on the node at path and not on its children. It
	// replaces the entry the grantee of e already has.
	AddNodeACL(ctx context.Context, path string, e ACLEntry) error
	// RemoveNodeACL removes the entry of the grantee of e from the node at path
	// and not from its children, whatever its permissions.
	RemoveNodeACL(ctx context.Context, path string, e ACLEntry) error
}

// ReconcilableFS is a storage driver the deep job can reconcile. The storage
// provider exposes it to the job through the invocations below.
type ReconcilableFS interface {
	storage.FS
	ReconcileMethods
}

// The invocations of a storage provider whose driver is a ReconcilableFS.
const (
	// InvocationDumpNamespace streams the dump of ArgPath, with at most
	// DumpBatchSize nodes under "nodes" in each result.
	InvocationDumpNamespace = "reconciliation.dump"
	// InvocationAddNodeACL and InvocationRemoveNodeACL take ArgPath and ArgEntry.
	InvocationAddNodeACL    = "reconciliation.addacl"
	InvocationRemoveNodeACL = "reconciliation.removeacl"

	ArgPath = "path"
	// ArgEntry holds an ACLEntry as JSON.
	ArgEntry = "entry"

	// DumpBatchSize keeps the number of messages of a dump low without making
	// one message too large for the transport.
	DumpBatchSize = 1000
)
