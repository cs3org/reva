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
	"cmp"
	"context"
	"fmt"
	"slices"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

const JobName = "reconciliation.deep"

type RunResult struct {
	Entries []EntryResult
}

type EntryResult struct {
	Path   string
	Action ActionKind
	ACL    acl.Entry
}

type DeepJob struct {
	shareMgr sql.ShareMgr
}

type RunParameters struct {
	SpaceID   string
	SpaceType spaces.SpaceType
}

type ShareWithPath struct {
	*collaborationv1beta1.Share
	Path string
}

func (s *ShareWithPath) toTreeNode() *ACLNode {
	return &ACLNode{
		Path:          s.Path,
		MandatoryACLs: []acl.Entry{shareToACL(s.Share)},
	}
}

func (j *DeepJob) run(ctx context.Context, p RunParameters) error {

	// First, construct a tree which contains the "ideal" state
	// For this, we:
	// 1. Get all the shares in the space
	// 2. Resolve all their paths
	// 3. Sort them (this makes the tree insertion much faster)
	// 4. Construct the tree
	shares, err := j.shareMgr.ListShares(ctx, []*collaborationv1beta1.Filter{
		{
			Type: collaborationv1beta1.Filter_TYPE_SPACE_ID,
			Term: &collaborationv1beta1.Filter_SpaceId{
				SpaceId: p.SpaceID,
			},
		},
	})
	if err != nil {
		return err
	}

	// Resolve the paths
	var sharesWithPaths = make([]*ShareWithPath, len(shares))
	for _, s := range shares {
		if p, ok := j.getPath(s.ResourceId); ok {
			sharesWithPaths = append(sharesWithPaths, &ShareWithPath{
				Share: s,
				Path:  p,
			})
		}
	}

	// Sort the shares,
	slices.SortFunc(sharesWithPaths, func(a, b *ShareWithPath) int {
		return cmp.Compare(a.Path, b.Path)
	})

	// Finally, construct the tree
	tree := NewACLTree()
	for _, s := range sharesWithPaths {
		tree.Insert(s.toTreeNode())
	}

	// Now that we have the ideal state, we need to get the
	// actual state of the namespace into something parsable.
	// We start by taking a dump of the namespace, and then we
	// parse this entry-by-entry

	namespaceDump, err := NewEOSMemoryNSInspect()
	if err != nil {
		return err
	}

	path, err := spaces.DecodeSpaceID(p.SpaceID)
	if err != nil {
		return err
	}

	ns, err := namespaceDump.Dump(path, 0)

	for _, entry := range ns.entries {
		fmt.Print(entry.Path)
	}

	return nil
}

func (j *DeepJob) getPath(rid *provider.ResourceId) (string, bool) {
	return "", true
}

func shareToACL(s *collaborationv1beta1.Share) acl.Entry {
	var e acl.Entry
	switch {
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP:
		e.Type = "egroup"
		e.Qualifier = s.Grantee.GetGroupId().GetOpaqueId()
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER:
		e.Type = "u"
		e.Qualifier = s.Grantee.GetUserId().GetOpaqueId()
	default:
		return e
	}

	// TODO(jgeens): set permissions
	return e
}
