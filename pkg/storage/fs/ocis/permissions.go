// Copyright 2018-2020 CERN
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

package ocis

import (
	"context"
	"syscall"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/pkg/xattr"
)

var defaultPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	// no permissions
}
var ownerPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	// all permissions
	AddGrant:             true,
	CreateContainer:      true,
	Delete:               true,
	GetPath:              true,
	GetQuota:             true,
	InitiateFileDownload: true,
	InitiateFileUpload:   true,
	ListContainer:        true,
	ListFileVersions:     true,
	ListGrants:           true,
	ListRecycle:          true,
	Move:                 true,
	PurgeRecycle:         true,
	RemoveGrant:          true,
	RestoreFileVersion:   true,
	RestoreRecycleItem:   true,
	Stat:                 true,
	UpdateGrant:          true,
}

/*
// TODO if user is owner but no acls found he can do everything?
// The owncloud driver does not integrate with the os so, for now, the owner can do everything, see ownerPermissions.
// Should this change we can store an acl for the owner in every node.
// We could also add default acls that can only the admin can set, eg for a read only storage?
// Someone needs to write to provide the content that should be read only, so this would likely be an acl for a group anyway.
// We need the storage relative path so we can calculate the permissions
// for the node based on all acls in the tree up to the root
func (fs *ocisfs) readPermissions(ctx context.Context, n *Node) (p *provider.ResourcePermissions, err error) {

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("no user in context, returning default permissions")
		return defaultPermissions, nil
	}
	// check if the current user is the owner
	if n.ownerID == u.Id.OpaqueId {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("user is owner, returning owner permissions")
		return ownerPermissions, nil
	}

	// for non owners this is a little more complicated:
	aggregatedPermissions := &provider.ResourcePermissions{}
	// add default permissions
	addPermissions(aggregatedPermissions, defaultPermissions)

	// determine root
	var rn *Node
	if rn, err = fs.pw.RootNode(ctx); err != nil {
		return
	}

	cn := n

	// for an efficient group lookup convert the list of groups to a map
	// groups are just strings ... groupnames ... or group ids ??? AAARGH !!!
	groupsMap := make(map[string]bool, len(u.Groups))
	for i := range u.Groups {
		groupsMap[u.Groups[i]] = true
	}

	var e *ace.ACE
	// for all segments, starting at the leaf
	for cn.ID != rn.ID {

		var attrs []string
		if attrs, err = xattr.List(np); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("ipath", np).Msg("error listing attributes")
			return nil, err
		}

		userace := sharePrefix + "u:" + u.Id.OpaqueId
		userFound := false
		for i := range attrs {
			// we only need the find the user once per node
			switch {
			case !userFound && attrs[i] == userace:
				e, err = fs.readACE(ctx, np, "u:"+u.Id.OpaqueId)
			case strings.HasPrefix(attrs[i], sharePrefix+"g:"):
				g := strings.TrimPrefix(attrs[i], sharePrefix+"g:")
				if groupsMap[g] {
					e, err = fs.readACE(ctx, np, "g:"+g)
				} else {
					// no need to check attribute
					continue
				}
			default:
				// no need to check attribute
				continue
			}

			switch {
			case err == nil:
				addPermissions(aggregatedPermissions, e.Grant().GetPermissions())
				appctx.GetLogger(ctx).Debug().Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Interface("permissions", aggregatedPermissions).Msg("adding permissions")
			case isNoData(err):
				err = nil
				appctx.GetLogger(ctx).Error().Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Interface("attrs", attrs).Msg("no permissions found on node, but they were listed")
			default:
				appctx.GetLogger(ctx).Error().Err(err).Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Msg("error reading permissions")
				return nil, err
			}
		}

		np = filepath.Dir(np)
	}

	// 3. read user permissions until one is found?
	//   what if, when checking /a/b/c/d, /a/b has write permission, but /a/b/c has not?
	//      those are two shares one read only, and a higher one rw,
	//       should the higher one be used?
	//       or, since we did find a matching ace in a lower node use that because it matches the principal?
	//		this would allow ai user to share a folder rm but take away the write capability for eg a docs folder inside it.
	// 4. read group permissions until all groups of the user are matched?
	//    same as for user permission, but we need to keep going further up the tree until all groups of the user were matched.
	//    what if a user has thousands of groups?
	//      we will always have to walk to the root.
	//      but the same problem occurs for a user with 2 groups but where only one group was used to share.
	//      in any case we need to iterate the aces, not the number of groups of the user.
	//        listing the aces can be used to match the principals, we do not need to fully real all aces
	//   what if, when checking /a/b/c/d, /a/b has write permission for group g, but /a/b/c has an ace for another group h the user is also a member of?
	//     it would allow restricting a users permissions by resharing something with him with lower permission?
	//     so if you have reshare permissions you could accidentially restrict users access to a subfolder of a rw share to ro by sharing it to another group as ro when they are part of both groups
	//       it makes more sense to have explicit negative permissions

	// TODO we need to read all parents ... until we find a matching ace?
	appctx.GetLogger(ctx).Debug().Interface("permissions", aggregatedPermissions).Str("ipath", ip).Msg("returning aggregated permissions")
	return aggregatedPermissions, nil
}
*/
func isNoData(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(syscall.Errno); ok2 {
			return serr == syscall.ENODATA
		}
	}
	return false
}

// The os not exists error is buried inside the xattr error,
// so we cannot just use os.IsNotExists().
func isNotFound(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(syscall.Errno); ok2 {
			return serr == syscall.ENOENT
		}
	}
	return false
}

func (fs *ocisfs) readACE(ctx context.Context, ip string, principal string) (e *ace.ACE, err error) {
	var b []byte
	if b, err = xattr.Get(ip, sharePrefix+principal); err != nil {
		return nil, err
	}
	if e, err = ace.Unmarshal(principal, b); err != nil {
		return nil, err
	}
	return
}

// additive merging of permissions only
func addPermissions(p1 *provider.ResourcePermissions, p2 *provider.ResourcePermissions) {
	p1.AddGrant = p1.AddGrant || p2.AddGrant
	p1.CreateContainer = p1.CreateContainer || p2.CreateContainer
	p1.Delete = p1.Delete || p2.Delete
	p1.GetPath = p1.GetPath || p2.GetPath
	p1.GetQuota = p1.GetQuota || p2.GetQuota
	p1.InitiateFileDownload = p1.InitiateFileDownload || p2.InitiateFileDownload
	p1.InitiateFileUpload = p1.InitiateFileUpload || p2.InitiateFileUpload
	p1.ListContainer = p1.ListContainer || p2.ListContainer
	p1.ListFileVersions = p1.ListFileVersions || p2.ListFileVersions
	p1.ListGrants = p1.ListGrants || p2.ListGrants
	p1.ListRecycle = p1.ListRecycle || p2.ListRecycle
	p1.Move = p1.Move || p2.Move
	p1.PurgeRecycle = p1.PurgeRecycle || p2.PurgeRecycle
	p1.RemoveGrant = p1.RemoveGrant || p2.RemoveGrant
	p1.RestoreFileVersion = p1.RestoreFileVersion || p2.RestoreFileVersion
	p1.RestoreRecycleItem = p1.RestoreRecycleItem || p2.RestoreRecycleItem
	p1.Stat = p1.Stat || p2.Stat
	p1.UpdateGrant = p1.UpdateGrant || p2.UpdateGrant
}
