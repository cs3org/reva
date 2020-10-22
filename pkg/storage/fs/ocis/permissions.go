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
	"strings"
	"syscall"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

var defaultPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	// no permissions
}

// permissions for nodes that don't have an owner set, eg the root node
var noOwnerPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	Stat: true,
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

// Permissions implements permission checks
type Permissions struct {
	lu *Lookup
}

// HasPermission call check() for every node up to the root until check returns true
func (p *Permissions) HasPermission(ctx context.Context, n *Node, check func(*provider.ResourcePermissions) bool) (can bool, err error) {

	var u *userv1beta1.User
	var perms *provider.ResourcePermissions
	if u, perms = p.getUserAndPermissions(ctx, n); perms != nil {
		return check(perms), nil
	}

	// determine root
	var rn *Node
	if rn, err = p.lu.RootNode(ctx); err != nil {
		return false, err
	}

	cn := n

	// for an efficient group lookup convert the list of groups to a map
	// groups are just strings ... groupnames ... or group ids ??? AAARGH !!!
	groupsMap := make(map[string]bool, len(u.Groups))
	for i := range u.Groups {
		groupsMap[u.Groups[i]] = true
	}

	var g *provider.Grant
	// for all segments, starting at the leaf
	for cn.ID != rn.ID {

		var grantees []string
		if grantees, err = cn.ListGrantees(ctx); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", cn).Msg("error listing grantees")
			return false, err
		}

		userace := grantPrefix + "u:" + u.Id.OpaqueId
		userFound := false
		for i := range grantees {
			// we only need the find the user once per node
			switch {
			case !userFound && grantees[i] == userace:
				g, err = cn.ReadGrant(ctx, grantees[i])
			case strings.HasPrefix(grantees[i], grantPrefix+"g:"):
				gr := strings.TrimPrefix(grantees[i], grantPrefix+"g:")
				if groupsMap[gr] {
					g, err = cn.ReadGrant(ctx, grantees[i])
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
				appctx.GetLogger(ctx).Debug().Interface("node", cn).Str("grant", grantees[i]).Interface("permissions", g.GetPermissions()).Msg("checking permissions")
				if check(g.GetPermissions()) {
					return true, nil
				}
			case isNoData(err):
				err = nil
				appctx.GetLogger(ctx).Error().Interface("node", cn).Str("grant", grantees[i]).Interface("grantees", grantees).Msg("grant vanished from node after listing")
			default:
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", cn).Str("grant", grantees[i]).Msg("error reading permissions")
				return false, err
			}
		}

		if cn, err = cn.Parent(); err != nil {
			return false, errors.Wrap(err, "ocisfs: error getting parent "+cn.ParentID)
		}
	}

	appctx.GetLogger(ctx).Debug().Interface("permissions", defaultPermissions).Interface("node", n).Interface("user", u).Msg("no grant found, returning default permissions")
	return false, nil
}

func (p *Permissions) getUserAndPermissions(ctx context.Context, n *Node) (*userv1beta1.User, *provider.ResourcePermissions) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("no user in context, returning default permissions")
		return nil, defaultPermissions
	}
	// check if the current user is the owner
	id, _, err := n.Owner()
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not determine owner, returning default permissions")
		return nil, defaultPermissions
	}
	if id == "" {
		// TODO what if no owner is set but grants are present?
		return nil, noOwnerPermissions
	}
	if id == u.Id.OpaqueId {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("user is owner, returning owner permissions")
		return u, ownerPermissions
	}
	return u, nil
}
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
