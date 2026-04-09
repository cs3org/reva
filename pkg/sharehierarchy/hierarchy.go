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

// Package sharehierarchy implements the hierarchical share consistency algorithm
// described in ADR-0005-P01. It ensures that when a share is created, updated, or
// deleted, the resulting set of EOS ACLs remains consistent with all outstanding
// shares for the same grantee in the same storage space.
package sharehierarchy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// Checker runs the hierarchical share consistency algorithm.
// GetPath must be populated before any method is called.
type Checker struct {
	// GetPath resolves a ResourceId to its current filesystem path.
	GetPath func(ctx context.Context, id *provider.ResourceId) (string, error)
}

// HierarchyCheckResult is the outcome of CheckForAdd.
// It contains lists of which shares should be re-applied and which ones should be deleted.
type HierarchyCheckResult struct {
	// ToDelete contains child shares that would become redundant or inconsistent.
	// When force=true these are populated and the caller must remove them.
	// When force=false and this slice would be non-empty, CheckForAdd returns
	// ShareChildConflict instead of populating this field.
	ToDelete []*collaboration.Share

	// ToReapply contains child shares whose ACLs must be explicitly re-applied after
	// adding the new ACL. This covers the (N=R, C=RW) case where the child has higher
	// permissions than the new parent share and must retain its explicit ACL.
	ToReapply []*collaboration.Share
}

// CheckGrantConsistency runs the two-step hierarchy check for creating or updating a share.
//
// It verifies that granting nodePerms on nodePath is consistent
// with all existing shares for the same grantee in the same space. It returns a
// HierarchyCheckResult describing which child shares must be deleted or re-applied,
// or a ShareParentConflict error if a parent share makes the grant invalid.
//
// nodePath is the current filesystem path of the resource being shared.
// nodePerms is the permission level being granted.
// existingShares contains all active shares for the same (spaceId, grantee), with
// the share being updated (if any) already excluded by the caller.
//
// Possible return values:
//   - (result, nil): operation can proceed. result.ToDelete and result.ToReapply
//     may be non-empty and must be acted on by the caller.
//   - (nil, ShareParentConflict): hard failure, operation must be aborted.
func (c *Checker) CheckGrantConsistency(ctx context.Context, nodePath string, nodePerms *provider.ResourcePermissions, existingShares []*collaboration.Share) (*HierarchyCheckResult, error) {
	result := &HierarchyCheckResult{}
	nodePermLevel := PermLevelFromCS3(nodePerms)
	reapplyPaths := make(map[string]string)

	for _, s := range existingShares {
		path, err := c.GetPath(ctx, s.ResourceId)
		if err != nil {
			// Share may be orphaned or its resource temporarily unavailable; skip it.
			continue
		}

		sharePerms := PermLevelFromCS3(s.Permissions.GetPermissions())

		switch {
		case isStrictAncestor(path, nodePath):
			// Step 1: existing share S is a parent of the new node N.
			// Allowed only when N strictly escalates beyond P in the permission ordering.
			// Any other combination means N is redundant or conflicts with P.
			if nodePermLevel > sharePerms {
				continue
			}
			return nil, errtypes.ShareParentConflict(fmt.Sprintf(
				"resource at %q is already accessible via a %s share on parent %q",
				nodePath, sharePerms, path,
			))

		case isStrictAncestor(nodePath, path):
			// Step 2: existing share S is a child of the new node N.
			// When the child has strictly higher permissions than N, its explicit ACL
			// must be re-applied after adding N so it is not shadowed.
			// Otherwise the child is redundant or would be implicitly elevated and must be deleted.
			if sharePerms > nodePermLevel {
				result.ToReapply = append(result.ToReapply, s)
				reapplyPaths[s.Id.OpaqueId] = path
			} else {
				result.ToDelete = append(result.ToDelete, s)
			}
		}
	}

	// Sort ToReapply shallowest-first so the caller can apply ACLs in the correct order.
	sortByPathDepthAsc(result.ToReapply, reapplyPaths)
	return result, nil
}

// RemoveReapplyResult holds the grants that must be re-applied after a share is removed.
type RemoveReapplyResult struct {
	// ParentGrant is the closest ancestor share, or nil if none exists.
	// Its permissions must be re-applied to the removed share's resource.
	ParentGrant *collaboration.Share
	// ChildGrants is the list of descendant shares sorted shallowest-first.
	// Each must be re-applied to its own resource.
	ChildGrants []*collaboration.Share
}

// GrantsToReapplyAfterRemove computes the grants that must be re-applied once the
// share identified by removedID on removedResourceID is deleted.
//
// allShares must contain all active shares for the same (spaceId, grantee), including
// the share being removed — this method excludes it internally.
//
// If the path of the removed resource cannot be resolved, both fields are nil/empty.
func (c *Checker) GrantsToReapplyAfterRemove(ctx context.Context, removedID string, removedResourceID *provider.ResourceId, allShares []*collaboration.Share) *RemoveReapplyResult {
	// Resolve paths for all shares except the one being removed.
	// Shares absent from the map are naturally excluded by filterAncestors/filterDescendants.
	paths := make(map[string]string, len(allShares))
	var removedPath string
	for _, s := range allShares {
		path, err := c.GetPath(ctx, s.ResourceId)
		if s.Id.OpaqueId == removedID {
			if err != nil {
				return &RemoveReapplyResult{}
			}
			removedPath = path
		} else if err == nil {
			paths[s.Id.OpaqueId] = path
		}
	}

	ancestors := filterAncestors(allShares, paths, removedPath)
	descendants := filterDescendants(allShares, paths, removedPath)
	sortByPathDepthAsc(descendants, paths)
	return &RemoveReapplyResult{
		ParentGrant: closestAncestor(ancestors, paths),
		ChildGrants: descendants,
	}
}

// filterAncestors returns the subset of shares whose resolved path is a strict
// ancestor of targetPath.
func filterAncestors(shares []*collaboration.Share, paths map[string]string, targetPath string) []*collaboration.Share {
	var result []*collaboration.Share
	for _, s := range shares {
		if p, ok := paths[s.Id.OpaqueId]; ok && isStrictAncestor(p, targetPath) {
			result = append(result, s)
		}
	}
	return result
}

// filterDescendants returns the subset of shares whose resolved path is a strict
// descendant of targetPath.
func filterDescendants(shares []*collaboration.Share, paths map[string]string, targetPath string) []*collaboration.Share {
	var result []*collaboration.Share
	for _, s := range shares {
		if p, ok := paths[s.Id.OpaqueId]; ok && isStrictAncestor(targetPath, p) {
			result = append(result, s)
		}
	}
	return result
}

// closestAncestor returns the ancestor share with the longest (most specific) path,
// i.e. the nearest parent. Returns nil if ancestors is empty.
func closestAncestor(ancestors []*collaboration.Share, paths map[string]string) *collaboration.Share {
	var closest *collaboration.Share
	var closestPath string
	for _, s := range ancestors {
		p := paths[s.Id.OpaqueId]
		if closest == nil || len(p) > len(closestPath) {
			closest = s
			closestPath = p
		}
	}
	return closest
}

// sortByPathDepthAsc sorts shares in place by ascending path depth (shallowest first).
// Applying ACLs in this order ensures parent permissions are set before child permissions,
// maintaining correct inheritance semantics.
func sortByPathDepthAsc(shares []*collaboration.Share, paths map[string]string) {
	sort.Slice(shares, func(i, j int) bool {
		pi := paths[shares[i].Id.OpaqueId]
		pj := paths[shares[j].Id.OpaqueId]
		return strings.Count(pi, string(os.PathSeparator)) < strings.Count(pj, string(os.PathSeparator))
	})
}

// isStrictAncestor returns true if ancestorPath is a proper prefix of childPath,
// i.e. every component of ancestorPath is a parent of childPath.
// "/a" is a strict ancestor of "/a/b" but not of "/a" itself or "/ab/c".
func isStrictAncestor(ancestorPath, childPath string) bool {
	rel, err := filepath.Rel(ancestorPath, childPath)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

// ChildConflictMessage builds a human-readable description of a child conflict
// for use in error responses. Called by the gateway when force=false and ToDelete is non-empty.
func ChildConflictMessage(shares []*collaboration.Share) string {
	descs := make([]string, 0, len(shares))
	for _, s := range shares {
		descs = append(descs, fmt.Sprintf("share %s (resource %s/%s)",
			s.Id.OpaqueId, s.ResourceId.StorageId, s.ResourceId.OpaqueId))
	}
	return fmt.Sprintf(
		"creating this share will delete %d child share(s): %s",
		len(shares), strings.Join(descs, ", "),
	)
}
