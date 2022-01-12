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

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocsconv "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/xattr"
)

const (
	spaceTypeAny = "*"
	spaceIDAny   = "*"
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
	// allow sending a space id
	if req.Opaque != nil && req.Opaque.Map != nil {
		if e, ok := req.Opaque.Map["spaceid"]; ok && e.Decoder == "plain" {
			spaceID = string(e.Value)
		}
	}
	// TODO enforce a uuid?
	// TODO clarify if we want to enforce a single personal storage space or if we want to allow sending the spaceid
	if req.Type == "personal" {
		spaceID = req.Owner.Id.OpaqueId
	}

	n, err := r.Child(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	if n.Exists {
		return nil, errtypes.AlreadyExists("decomposedfs: spaces: space already exists")
	}

	// spaceid and nodeid must be the same
	// TODO enforce a uuid?
	n.ID = spaceID

	if err := fs.tp.CreateDir(ctx, n); err != nil {
		return nil, err
	}

	// always enable propagation on the storage space root
	nodePath := n.InternalPath()
	// mark the space root node as the end of propagation
	if err = xattr.Set(nodePath, xattrs.PropagationAttr, []byte("1")); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not mark node to propagate")
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

	if q := req.GetQuota(); q != nil {
		// set default space quota
		if err := n.SetMetadata(xattrs.QuotaAttr, strconv.FormatUint(q.QuotaMaxBytes, 10)); err != nil {
			return nil, err
		}
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
			Root: &provider.ResourceId{
				StorageId: spaceID,
				OpaqueId:  spaceID,
			},
			Name:      req.GetName(),
			Quota:     req.GetQuota(),
			SpaceType: req.GetType(),
		},
	}

	ctx = context.WithValue(ctx, SpaceGrant, struct{}{})

	if err := fs.AddGrant(ctx, &provider.Reference{
		ResourceId: resp.StorageSpace.Root,
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
func (fs *Decomposedfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter, permissions map[string]struct{}) ([]*provider.StorageSpace, error) {
	// TODO check filters

	// TODO when a space symlink is broken delete the space for cleanup
	// read permissions are deduced from the node?

	// TODO for absolute references this actually requires us to move all user homes into a subfolder of /nodes/root,
	// e.g. /nodes/root/<space type> otherwise storage space names might collide even though they are of different types
	// /nodes/root/personal/foo and /nodes/root/shares/foo might be two very different spaces, a /nodes/root/foo is not expressive enough
	// we would not need /nodes/root if access always happened via spaceid+relative path

	var (
		spaceID = spaceIDAny
		nodeID  = spaceIDAny
	)

	spaceTypes := []string{}

	for i := range filter {
		switch filter[i].Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			switch filter[i].GetSpaceType() {
			case "+mountpoint":
				// TODO include mount poits
			case "+grant":
				// TODO include grants
			default:
				spaceTypes = append(spaceTypes, filter[i].GetSpaceType())
			}
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			spaceID, nodeID, _ = utils.SplitStorageSpaceID(filter[i].GetId().OpaqueId)
		}
	}
	if len(spaceTypes) == 0 {
		spaceTypes = []string{"*"}
	}

	spaces := []*provider.StorageSpace{}
	// build the glob path, eg.
	// /path/to/root/spaces/{spaceType}/{spaceId}
	// /path/to/root/spaces/personal/nodeid
	// /path/to/root/spaces/shared/nodeid

	matches := []string{}
	for _, spaceType := range spaceTypes {
		m, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceType, nodeID))
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}

	// FIXME if the space does not exist try a node as the space root.

	// But then the whole /spaces/{spaceType}/{spaceid} becomes obsolete
	// we can alway just look up by nodeid
	// -> no. The /spaces folder is used for efficient lookup by type, otherwise we would have
	//    to iterate over all nodes and read the type from extended attributes
	// -> but for lookup by id we can use the node directly.
	// But what about sharding nodes by space?
	// an efficient lookup would be possible if we received a spaceid&opaqueid in the request
	// the personal spaces must also use the nodeid and not the name

	numShares := 0
	for i := range matches {
		var target string
		var err error
		// always read link in case storage space id != node id
		if target, err = os.Readlink(matches[i]); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[i]).Msg("could not read link, skipping")
			continue
		}

		n, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("id", filepath.Base(target)).Msg("could not read node, skipping")
			continue
		}

		spaceType := filepath.Base(filepath.Dir(matches[i]))

		// FIXME type share evolved to grant on the edge branch ... make it configurable if the driver should support them or not for now ... ignore type share
		if spaceType == "share" {
			numShares++
			// do not list shares as spaces for the owner
			continue
		}

		// TODO apply more filters
		space, err := fs.storageSpaceFromNode(ctx, n, spaceType, matches[i], permissions)
		if err != nil {
			if _, ok := err.(errtypes.IsPermissionDenied); !ok {
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not convert to storage space")
			}
			continue
		}
		spaces = append(spaces, space)

	}
	// if there are no matches (or they happened to be spaces for the owner) and the node is a child return a space
	if len(matches) <= numShares && nodeID != spaceID {
		// try node id
		target := filepath.Join(fs.o.Root, "nodes", nodeID)
		n, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
		if err != nil {
			return nil, err
		}
		if n.Exists {
			space, err := fs.storageSpaceFromNode(ctx, n, "*", n.InternalPath(), permissions)
			if err != nil {
				return nil, err
			}
			spaces = append(spaces, space)
		}
	}

	return spaces, nil

}

// UpdateStorageSpace updates a storage space
func (fs *Decomposedfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	space := req.StorageSpace

	_, spaceID, _ := utils.SplitStorageSpaceID(space.Id.OpaqueId)

	matches, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceTypeAny, spaceID))
	if err != nil {
		return nil, err
	}

	if len(matches) != 1 {
		return &provider.UpdateStorageSpaceResponse{
			Status: &v1beta11.Status{
				Code:    v1beta11.Code_CODE_NOT_FOUND,
				Message: fmt.Sprintf("update space failed: found %d matching spaces", len(matches)),
			},
		}, nil
	}

	target, err := os.Readlink(matches[0])
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[0]).Msg("could not read link, skipping")
	}

	node, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
	if err != nil {
		return nil, err
	}

	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("decomposedfs: spaces: contextual user not found")
	}
	space.Owner = u

	if space.Name != "" {
		if err := node.SetMetadata(xattrs.SpaceNameAttr, space.Name); err != nil {
			return nil, err
		}
	}

	if space.Quota != nil {
		if err := node.SetMetadata(xattrs.QuotaAttr, strconv.FormatUint(space.Quota.QuotaMaxBytes, 10)); err != nil {
			return nil, err
		}
	}

	return &provider.UpdateStorageSpaceResponse{
		Status:       &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		StorageSpace: space,
	}, nil
}

// DeleteStorageSpace deletes a storage space
func (fs *Decomposedfs) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) error {
	spaceID := req.Id.OpaqueId

	matches, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceTypeAny, spaceID))
	if err != nil {
		return err
	}

	if len(matches) != 1 {
		return fmt.Errorf("update space failed: found %d matching spaces", len(matches))
	}

	target, err := os.Readlink(matches[0])
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[0]).Msg("could not read link, skipping")
	}

	node, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
	if err != nil {
		return err
	}

	err = fs.tp.Delete(ctx, node)
	if err != nil {
		return err
	}

	err = os.RemoveAll(matches[0])
	if err != nil {
		return err
	}
	return nil
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

func (fs *Decomposedfs) createStorageSpace(ctx context.Context, spaceType, spaceID string) error {
	// create space type dir
	if err := os.MkdirAll(filepath.Join(fs.o.Root, "spaces", spaceType), 0700); err != nil {
		return err
	}

	// we can reuse the node id as the space id
	err := os.Symlink("../../nodes/"+spaceID, filepath.Join(fs.o.Root, "spaces", spaceType, spaceID))
	if err != nil {
		if isAlreadyExists(err) {
			appctx.GetLogger(ctx).Debug().Err(err).Str("space", spaceID).Str("spacetype", spaceType).Msg("symlink already exists")
		} else {
			// TODO how should we handle error cases here?
			appctx.GetLogger(ctx).Error().Err(err).Str("space", spaceID).Str("spacetype", spaceType).Msg("could not create symlink")
		}
	}

	return nil
}

func (fs *Decomposedfs) storageSpaceFromNode(ctx context.Context, n *node.Node, spaceType, nodePath string, permissions map[string]struct{}) (*provider.StorageSpace, error) {
	owner, err := n.Owner()
	if err != nil {
		return nil, err
	}

	// TODO apply more filters

	sname := ""
	if bytes, err := xattr.Get(n.InternalPath(), xattrs.SpaceNameAttr); err == nil {
		sname = string(bytes)
	}

	if err := n.FindStorageSpaceRoot(); err != nil {
		return nil, err
	}

	glob := filepath.Join(fs.o.Root, "spaces", spaceType, n.SpaceRoot.ID)
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}

	if len(matches) != 1 {
		return nil, errtypes.InternalError("expected only one match for " + glob)
	}

	spaceType = filepath.Base(filepath.Dir(matches[0]))

	space := &provider.StorageSpace{
		Id: &provider.StorageSpaceId{OpaqueId: n.SpaceRoot.ID},
		Root: &provider.ResourceId{
			StorageId: n.SpaceRoot.ID,
			OpaqueId:  n.SpaceRoot.ID,
		},
		Name:      sname,
		SpaceType: spaceType,
		// Mtime is set either as node.tmtime or as fi.mtime below
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	_, canListAllSpaces := permissions["list-all-spaces"]
	if !canListAllSpaces {
		ok, err := node.NewPermissions(fs.lu).HasPermission(ctx, n, func(p *provider.ResourcePermissions) bool {
			return p.Stat
		})
		if err != nil || !ok {
			return nil, errtypes.PermissionDenied(fmt.Sprintf("user %s is not allowed to Stat the space %+v", user.Username, space))
		}
	}

	space.Owner = &userv1beta1.User{ // FIXME only return a UserID, not a full blown user object
		Id: owner,
	}

	// we set the space mtime to the root item mtime
	// override the stat mtime with a tmtime if it is present
	if tmt, err := n.GetTMTime(); err == nil {
		un := tmt.UnixNano()
		space.Mtime = &types.Timestamp{
			Seconds: uint64(un / 1000000000),
			Nanos:   uint32(un % 1000000000),
		}
	} else if fi, err := os.Stat(nodePath); err == nil {
		// fall back to stat mtime
		un := fi.ModTime().UnixNano()
		space.Mtime = &types.Timestamp{
			Seconds: uint64(un / 1000000000),
			Nanos:   uint32(un % 1000000000),
		}
	}

	// quota
	v, err := xattr.Get(nodePath, xattrs.QuotaAttr)
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
			return nil, err
		}
	}

	return space, nil
}
