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
	"slices"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/reconciliation/nsdump"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/acl"
	"github.com/pkg/errors"
)

// TODO(jgeens):
// - check what to do with version folders

const JobName = "reconciliation.deep"

type DeepJob struct {
	shareMgr ShareStore
	gw       gateway.GatewayAPIClient
}

type ChangeSet []*Change

type Change struct {
	Path   string
	Action ActionKind
	ACL    *acl.Entry
}

type RunParameters struct {
	SpaceID       string
	SpaceType     spaces.SpaceType
	OptionalACLs  []*acl.Entry
	MandatoryACLs []*acl.Entry
}

type ShareWithPath struct {
	*collaborationv1beta1.Share
	Path string
}

func (s *ShareWithPath) toTreeNode() *ACLNode {
	return &ACLNode{
		Path:          s.Path,
		MandatoryACLs: []*acl.Entry{shareToACL(s.Share)},
	}
}

func (j *DeepJob) Run(ctx context.Context, p RunParameters) error {
	path, err := spaces.DecodeSpaceID(p.SpaceID)
	if err != nil {
		return err
	}

	statRes, err := j.gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{
		Path: path,
	}})

	if err != nil {
		return err
	}

	// TODO(jgeens): check statRes status code

	namespaceDumper := &nsdump.EOSMemoryNSInspect{}
	err = namespaceDumper.Setup(nsdump.EOSMemoryNSInspectConfig{
		Instance: statRes.Info.Id.StorageId,
	})
	if err != nil {
		return err
	}

	// Now, we check things based on the type of space:
	switch p.SpaceType {
	case spaces.SpaceTypeHome:
		p.MandatoryACLs = append(p.MandatoryACLs, &acl.Entry{
			Type:        "user",
			Qualifier:   statRes.Info.Owner.OpaqueId,
			Permissions: "rwx",
		})
	case spaces.SpaceTypeProject:

	case spaces.SpaceTypePublic:
		return errors.New("deep reconciliation is not supported for public spaces")
	}

	_, err = j.runAnalysis(ctx, p.SpaceID, namespaceDumper)
	return err
}

func (j *DeepJob) runAnalysis(ctx context.Context, spaceid string, nsdumper nsdump.NSDumpClient) (ChangeSet, error) {

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
				SpaceId: spaceid,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Resolve the paths
	var sharesWithPaths = make([]*ShareWithPath, 0, len(shares))
	for _, s := range shares {
		if p, ok := j.getPath(ctx, s.ResourceId); ok {
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
	path, err := spaces.DecodeSpaceID(spaceid)
	if err != nil {
		return nil, err
	}

	dump, err := nsdumper.Dump(path, 0)
	if err != nil {
		return nil, err
	}

	// Now we do the diff and get the resulting ChangeSet
	changeSet := compare(tree, dump)

	return changeSet, nil
}

func (j *DeepJob) getPath(ctx context.Context, rid *provider.ResourceId) (string, bool) {
	statRes, err := j.gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: rid},
	})
	if err != nil {
		return "", false
	}
	if statRes.Status == nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		return "", false
	}
	return statRes.Info.Path, true
}

func compare(tree *ACLTree, ns *nsdump.NamespaceDump) ChangeSet {
	changeSet := ChangeSet{}
	for _, e := range ns.Entries {
		changeSet = append(changeSet, compareEntry(tree, e)...)
	}
	return changeSet
}

func compareEntry(tree *ACLTree, entry nsdump.NameSpaceEntry) ChangeSet {
	m, o, ok := tree.Find(entry.Path)
	if !ok {
		return nil
	}

	actualACLs := parseACLs(entry.XattrSysAcl)
	return calculateChangeSet(m, o, actualACLs, entry.Path)

}

func parseACLs(sysattr string) []*acl.Entry {
	acls, err := acl.Parse(sysattr, acl.ShortTextForm)
	if err != nil {
		return nil
	}
	return acls.Entries
}

func calculateChangeSet(mandatory, optional, actual []*acl.Entry, p string) ChangeSet {
	changeSet := ChangeSet{}

	// First calculate missing entries on `actual`
	for _, e := range mandatory {
		if !aclSetContains(actual, e) {
			changeSet = append(changeSet, &Change{
				Path:   p,
				Action: ActionAdd,
				ACL:    e,
			})
		}
	}

	// Then calculate which entries should not be there
	for _, e := range actual {
		if !aclSetContains(mandatory, e) && !aclSetContains(optional, e) {
			changeSet = append(changeSet, &Change{
				Path:   p,
				Action: ActionDelete,
				ACL:    e,
			})
		}
	}

	return changeSet
}

// We cannot use slices.Contains, because we want to compare
// the actual values, not the pointers
func aclSetContains(set []*acl.Entry, entry *acl.Entry) bool {
	return slices.ContainsFunc(set, func(c *acl.Entry) bool {
		return *c == *entry
	})
}

func shareToACL(s *collaborationv1beta1.Share) *acl.Entry {
	var e acl.Entry
	switch {
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP:
		e.Type = "egroup"
		e.Qualifier = s.Grantee.GetGroupId().GetOpaqueId()
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER:
		e.Type = "u"
		e.Qualifier = s.Grantee.GetUserId().GetOpaqueId()
	default:
		return &e
	}

	// TODO(jgeens): we should define named constants for these int values
	// TODO(jgeens): we should define named constants for the EOS perms
	ocs := permissions.OCSFromCS3Permission(s.Permissions.Permissions)
	switch ocs {
	case 0:
		e.Permissions = "!r!w!x"
	case 1:
		e.Permissions = "rx"
	case 15:
		e.Permissions = "rwx"
	}

	return &e
}
