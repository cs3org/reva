// Copyright 2018-2021 CERN
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

package decomposedfs

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocsconv "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// CreateStorageSpace creates a storage space
func (fs *Decomposedfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	// spaces will be located by default in the root of the storage.
	r, err := fs.lu.RootNode(ctx)
	if err != nil {
		return nil, err
	}

	// "everything is a resource" this is the unique ID for the Space resource.
	spaceID := uuid.New().String()

	n, err := r.Child(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	if n.Exists {
		return nil, fmt.Errorf("decomposedfs: spaces: invalid duplicated node with id `%s`", n.ID)
	}

	if err := fs.tp.CreateDir(ctx, n); err != nil {
		return nil, err
	}

	if err := fs.createHiddenSpaceFolder(ctx, n); err != nil {
		return nil, err
	}

	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("decomposedfs: spaces: contextual user not found")
	}

	if err := n.ChangeOwner(u.Id); err != nil {
		return nil, err
	}

	err = fs.createStorageSpace(ctx, req.Type, n.ID)
	if err != nil {
		return nil, err
	}

	// set default space quota
	if err := n.SetMetadata(xattrs.QuotaAttr, strconv.FormatUint(req.GetQuota().QuotaMaxBytes, 10)); err != nil {
		return nil, err
	}

	if err := n.SetMetadata(xattrs.SpaceNameAttr, req.Name); err != nil {
		return nil, err
	}

	resp := &provider.CreateStorageSpaceResponse{
		Status: &v1beta11.Status{
			Code: v1beta11.Code_CODE_OK,
		},
		StorageSpace: &provider.StorageSpace{
			Owner: u,
			Id: &provider.StorageSpaceId{
				OpaqueId: spaceID,
			},
			// TODO we have to omit Root information because the storage driver does not know its mount point.
			// Root: &provider.ResourceId{
			//	StorageId: "",
			//	OpaqueId:  "",
			// },
			Name:      req.GetName(),
			Quota:     req.GetQuota(),
			SpaceType: req.GetType(),
		},
	}

	nPath, err := fs.lu.Path(ctx, n)
	if err != nil {
		return nil, errors.Wrap(err, "decomposedfs: spaces: could not create space. invalid node path")
	}

	ctx = context.WithValue(ctx, SpaceGrant, struct{}{})

	if err := fs.AddGrant(ctx, &provider.Reference{
		Path: nPath,
	}, &provider.Grant{
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: u.Id,
			},
		},
		Permissions: ocsconv.NewManagerRole().CS3ResourcePermissions(),
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

// ListStorageSpaces returns a list of StorageSpaces.
// The list can be filtered by space type or space id.
// Spaces are persisted with symlinks in /spaces/<type>/<spaceid> pointing to ../../nodes/<nodeid>, the root node of the space
// The spaceid is a concatenation of storageid + "!" + nodeid
func (fs *Decomposedfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	// TODO check filters

	// TODO when a space symlink is broken delete the space for cleanup
	// read permissions are deduced from the node?

	// TODO for absolute references this actually requires us to move all user homes into a subfolder of /nodes/root,
	// e.g. /nodes/root/<space type> otherwise storage space names might collide even though they are of different types
	// /nodes/root/personal/foo and /nodes/root/shares/foo might be two very different spaces, a /nodes/root/foo is not expressive enough
	// we would not need /nodes/root if access always happened via spaceid+relative path

	var (
		spaceType = "*"
		spaceID   = "*"
	)

	for i := range filter {
		switch filter[i].Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			spaceType = filter[i].GetSpaceType()
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			parts := strings.SplitN(filter[i].GetId().OpaqueId, "!", 2)
			if len(parts) == 2 {
				spaceID = parts[1]
			}
		}
	}

	// build the glob path, eg.
	// /path/to/root/spaces/personal/nodeid
	// /path/to/root/spaces/shared/nodeid
	matches, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceType, spaceID))
	if err != nil {
		return nil, err
	}

	spaces := make([]*provider.StorageSpace, 0, len(matches))

	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		appctx.GetLogger(ctx).Debug().Msg("expected user in context")
		return spaces, nil
	}

	for i := range matches {
		// always read link in case storage space id != node id
		if target, err := os.Readlink(matches[i]); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[i]).Msg("could not read link, skipping")
			continue
		} else {
			n, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Str("id", filepath.Base(target)).Msg("could not read node, skipping")
				continue
			}
			owner, err := n.Owner()
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not read owner, skipping")
				continue
			}

			// TODO apply more filters

			space := &provider.StorageSpace{
				// FIXME the driver should know its id move setting the spaceid from the storage provider to the drivers
				//Id: &provider.StorageSpaceId{OpaqueId: "1284d238-aa92-42ce-bdc4-0b0000009157!" + n.ID},
				Root: &provider.ResourceId{
					// FIXME the driver should know its id move setting the spaceid from the storage provider to the drivers
					//StorageId: "1284d238-aa92-42ce-bdc4-0b0000009157",
					OpaqueId: n.ID,
				},
				Name:      n.Name,
				SpaceType: filepath.Base(filepath.Dir(matches[i])),
				// Mtime is set either as node.tmtime or as fi.mtime below
			}

			switch space.SpaceType {
			case "share":
				if utils.UserEqual(u.Id, owner) {
					// do not list shares as spaces for the owner
					continue
				}
			case "project":
				sname, err := xattr.Get(n.InternalPath(), xattrs.SpaceNameAttr)
				if err != nil {
					appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not read space name, attribute not found")
					continue
				}
				space.Name = string(sname)
			default:
				space.Name = "root"
			}

			// filter out spaces user cannot access (currently based on stat permission)
			p, err := n.ReadUserPermissions(ctx, u)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not read permissions, skipping")
				continue
			}
			if !p.Stat {
				continue
			}

			// fill in user object if the current user is the owner
			if utils.UserEqual(u.Id, owner) {
				space.Owner = u
			} else {
				space.Owner = &userv1beta1.User{ // FIXME only return a UserID, not a full blown user object
					Id: owner,
				}
			}

			// we set the space mtime to the root item mtime
			// override the stat mtime with a tmtime if it is present
			if tmt, err := n.GetTMTime(); err == nil {
				un := tmt.UnixNano()
				space.Mtime = &types.Timestamp{
					Seconds: uint64(un / 1000000000),
					Nanos:   uint32(un % 1000000000),
				}
			} else if fi, err := os.Stat(matches[i]); err == nil {
				// fall back to stat mtime
				un := fi.ModTime().UnixNano()
				space.Mtime = &types.Timestamp{
					Seconds: uint64(un / 1000000000),
					Nanos:   uint32(un % 1000000000),
				}
			}

			// quota
			v, err := xattr.Get(matches[i], xattrs.QuotaAttr)
			if err == nil {
				// make sure we have a proper signed int
				// we use the same magic numbers to indicate:
				// -1 = uncalculated
				// -2 = unknown
				// -3 = unlimited
				if quota, err := strconv.ParseUint(string(v), 10, 64); err == nil {
					space.Quota = &provider.Quota{
						QuotaMaxBytes: quota,
						QuotaMaxFiles: math.MaxUint64, // TODO MaxUInt64? = unlimited? why even max files? 0 = unlimited?
					}
				} else {
					appctx.GetLogger(ctx).Debug().Err(err).Str("nodepath", matches[i]).Msg("could not read quota")
				}
			}

			spaces = append(spaces, space)
		}
	}

	return spaces, nil

}

// createHiddenSpaceFolder bootstraps a storage space root with a hidden ".space" folder used to store space related
// metadata such as a description or an image.
// Internally createHiddenSpaceFolder leverages the use of node.Child() to create a new node under the space root.
// createHiddenSpaceFolder is just a contextual alias for node.Child() for ".spaces".
func (fs *Decomposedfs) createHiddenSpaceFolder(ctx context.Context, r *node.Node) error {
	hiddenSpace, err := r.Child(ctx, ".space")
	if err != nil {
		return err
	}

	return fs.tp.CreateDir(ctx, hiddenSpace)
}

func (fs *Decomposedfs) createStorageSpace(ctx context.Context, spaceType, nodeID string) error {
	// create space type dir
	if err := os.MkdirAll(filepath.Join(fs.o.Root, "spaces", spaceType), 0700); err != nil {
		return err
	}

	// we can reuse the node id as the space id
	err := os.Symlink("../../nodes/"+nodeID, filepath.Join(fs.o.Root, "spaces", spaceType, nodeID))
	if err != nil {
		if isAlreadyExists(err) {
			appctx.GetLogger(ctx).Debug().Err(err).Str("node", nodeID).Str("spacetype", spaceType).Msg("symlink already exists")
		} else {
			// TODO how should we handle error cases here?
			appctx.GetLogger(ctx).Error().Err(err).Str("node", nodeID).Str("spacetype", spaceType).Msg("could not create symlink")
		}
	}

	return nil
}

// containedWithinSpace is a working name. It checks whether a reference is contained within a Storage Space or not. If
// it is, it will return the root node of the storage space.
func (fs *Decomposedfs) containedWithinSpace(ctx context.Context, n *node.Node) (*node.Node, bool) {
	// get home or root node
	// is the current node's parent the root of a storage space?
	//	yes -> return the current node's parent and true
	//	no -> traverse up the tree until the home / root node checking for storage space's root nodes.
	root, err := fs.lu.HomeOrRootNode(ctx)
	if err != nil {
		return nil, false
	}

	if root.ID == n.ID {
		// we reached the root node, and by definition it is not a space.
		return nil, false
	}

	p, err := n.Parent()
	if err != nil {
		return nil, false
	}

	sName, err := xattr.Get(p.InternalPath(), xattrs.SpaceNameAttr)
	if err != nil {
		return nil, false
	}

	if len(sName) > 0 {
		return p, true
	}

	return fs.containedWithinSpace(ctx, p)
}
