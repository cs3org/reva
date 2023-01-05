// Copyright 2018-2023 CERN
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
	permissionsv1beta1 "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocsconv "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	spaceTypeAny = "*"
	spaceIDAny   = "*"
)

// CreateStorageSpace creates a storage space.
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
// The spaceid is a concatenation of storageid + "!" + nodeid.
func (fs *Decomposedfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	// TODO check filters

	// TODO when a space symlink is broken delete the space for cleanup
	// read permissions are deduced from the node?

	// TODO for absolute references this actually requires us to move all user homes into a subfolder of /nodes/root,
	// e.g. /nodes/root/<space type> otherwise storage space names might collide even though they are of different types
	// /nodes/root/personal/foo and /nodes/root/shares/foo might be two very different spaces, a /nodes/root/foo is not expressive enough
	// we would not need /nodes/root if access always happened via spaceid+relative path

	var (
		spaceType = spaceTypeAny
		spaceID   = spaceIDAny
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

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.o.GatewayAddr))
	if err != nil {
		return nil, err
	}

	checkRes, err := client.CheckPermission(ctx, &permissionsv1beta1.CheckPermissionRequest{
		Permission: "list-all-spaces",
		SubjectRef: &permissionsv1beta1.SubjectReference{
			Spec: &permissionsv1beta1.SubjectReference_UserId{
				UserId: u.Id,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	canListAllSpaces := false
	if checkRes.Status.Code == v1beta11.Code_CODE_OK {
		canListAllSpaces = true
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

			spaceType := filepath.Base(filepath.Dir(matches[i]))

			owner, err := n.Owner()
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not read owner, skipping")
				continue
			}

			if spaceType == "share" && utils.UserEqual(u.Id, owner) {
				// do not list shares as spaces for the owner
				continue
			}

			// TODO apply more filters
			space, err := fs.storageSpaceFromNode(ctx, n, matches[i], spaceType, canListAllSpaces)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not convert to storage space")
				continue
			}
			spaces = append(spaces, space)
		}
	}

	return spaces, nil
}

// UpdateStorageSpace updates a storage space.
func (fs *Decomposedfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	space := req.StorageSpace

	_, spaceID, err := utils.SplitStorageSpaceID(space.Id.OpaqueId)
	if err != nil {
		return nil, err
	}

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

func (fs *Decomposedfs) storageSpaceFromNode(ctx context.Context, node *node.Node, nodePath, spaceType string, canListAllSpaces bool) (*provider.StorageSpace, error) {
	owner, err := node.Owner()
	if err != nil {
		return nil, err
	}

	// TODO apply more filters

	sname, err := xattr.Get(node.InternalPath(), xattrs.SpaceNameAttr)
	if err != nil {
		return nil, err
	}
	space := &provider.StorageSpace{
		// FIXME the driver should know its id move setting the spaceid from the storage provider to the drivers
		//Id: &provider.StorageSpaceId{OpaqueId: "1284d238-aa92-42ce-bdc4-0b0000009157!" + n.ID},
		Root: &provider.ResourceId{
			// FIXME the driver should know its id move setting the spaceid from the storage provider to the drivers
			//StorageId: "1284d238-aa92-42ce-bdc4-0b0000009157",
			OpaqueId: node.ID,
		},
		Name:      string(sname),
		SpaceType: spaceType,
		// Mtime is set either as node.tmtime or as fi.mtime below
	}

	user := ctxpkg.ContextMustGetUser(ctx)

	// filter out spaces user cannot access (currently based on stat permission)
	if !canListAllSpaces {
		p, err := node.ReadUserPermissions(ctx, user)
		if err != nil {
			return nil, err
		}
		if !p.Stat {
			return nil, errors.New("user is not allowed to Stat the space")
		}
	}

	space.Owner = &userv1beta1.User{ // FIXME only return a UserID, not a full blown user object
		Id: owner,
	}

	// we set the space mtime to the root item mtime
	// override the stat mtime with a tmtime if it is present
	if tmt, err := node.GetTMTime(); err == nil {
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
