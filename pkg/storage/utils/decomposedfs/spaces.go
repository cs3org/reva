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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	permissionsv1beta1 "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocsconv "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
)

const (
	spaceTypePersonal = "personal"
	spaceTypeProject  = "project"
	spaceTypeShare    = "share"
	spaceTypeAny      = "*"
	spaceIDAny        = "*"
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
	// allow sending a space description
	var description string
	if req.Opaque != nil && req.Opaque.Map != nil {
		if e, ok := req.Opaque.Map["description"]; ok && e.Decoder == "plain" {
			description = string(e.Value)
		}
	}
	// TODO enforce a uuid?
	// TODO clarify if we want to enforce a single personal storage space or if we want to allow sending the spaceid
	if req.Type == spaceTypePersonal {
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
	// mark the space root node as the end of propagation
	if err = n.SetMetadata(xattrs.PropagationAttr, "1"); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not mark node to propagate")
		return nil, err
	}

	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("decomposedfs: spaces: contextual user not found")
	}

	ownerID := u.Id
	if req.Type == spaceTypeProject {
		ownerID = &userv1beta1.UserId{}
	}

	if err := n.ChangeOwner(ownerID); err != nil {
		return nil, err
	}

	err = fs.createStorageSpace(ctx, req.Type, n.ID)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string, 3)
	if q := req.GetQuota(); q != nil {
		// set default space quota
		metadata[xattrs.QuotaAttr] = strconv.FormatUint(q.QuotaMaxBytes, 10)
	}

	metadata[xattrs.SpaceNameAttr] = req.Name
	if description != "" {
		metadata[xattrs.SpaceDescriptionAttr] = description
	}
	if err := xattrs.SetMultiple(n.InternalPath(), metadata); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, utils.SpaceGrant, struct{}{})

	if err := fs.AddGrant(ctx, &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: spaceID,
			OpaqueId:  spaceID,
		},
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

	space, err := fs.storageSpaceFromNode(ctx, n, "*", n.InternalPath(), false)
	if err != nil {
		return nil, err
	}

	resp := &provider.CreateStorageSpaceResponse{
		Status: &v1beta11.Status{
			Code: v1beta11.Code_CODE_OK,
		},
		StorageSpace: space,
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
			if strings.Contains(nodeID, "/") {
				return []*provider.StorageSpace{}, nil
			}
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
		path := filepath.Join(fs.o.Root, "spaces", spaceType, nodeID)
		m, err := filepath.Glob(path)
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
	client, err := pool.GetGatewayServiceClient(fs.o.GatewayAddr)
	if err != nil {
		return nil, err
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	checkRes, err := client.CheckPermission(ctx, &permissionsv1beta1.CheckPermissionRequest{
		Permission: "list-all-spaces",
		SubjectRef: &permissionsv1beta1.SubjectReference{
			Spec: &permissionsv1beta1.SubjectReference_UserId{
				UserId: user.Id,
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
		if spaceType == spaceTypeShare {
			numShares++
			// do not list shares as spaces for the owner
			continue
		}

		// TODO apply more filters
		space, err := fs.storageSpaceFromNode(ctx, n, spaceType, matches[i], canListAllSpaces)
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
			space, err := fs.storageSpaceFromNode(ctx, n, "*", n.InternalPath(), canListAllSpaces)
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
	var restore bool
	if req.Opaque != nil {
		_, restore = req.Opaque.Map["restore"]
	}

	space := req.StorageSpace
	_, spaceID, _ := utils.SplitStorageSpaceID(space.Id.OpaqueId)

	if restore {
		matches, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceTypeAny, spaceID))
		if err != nil {
			return nil, err
		}

		if len(matches) != 1 {
			return &provider.UpdateStorageSpaceResponse{
				Status: &v1beta11.Status{
					Code:    v1beta11.Code_CODE_NOT_FOUND,
					Message: fmt.Sprintf("restoring space failed: found %d matching spaces", len(matches)),
				},
			}, nil

		}

		target, err := os.Readlink(matches[0])
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[0]).Msg("could not read link, skipping")
		}

		n, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
		if err != nil {
			return nil, err
		}

		newnode := *n
		newnode.Name = strings.Split(n.Name, node.TrashIDDelimiter)[0]
		newnode.Exists = false

		err = fs.tp.Move(ctx, n, &newnode)
		if err != nil {
			return nil, err
		}
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

	metadata := make(map[string]string, 5)
	if space.Name != "" {
		metadata[xattrs.SpaceNameAttr] = space.Name
	}

	if space.Quota != nil {
		metadata[xattrs.QuotaAttr] = strconv.FormatUint(space.Quota.QuotaMaxBytes, 10)
	}

	// TODO also return values which are not in the request
	hasDescription := false
	if space.Opaque != nil {
		if description, ok := space.Opaque.Map["description"]; ok {
			metadata[xattrs.SpaceDescriptionAttr] = string(description.Value)
			hasDescription = true
		}
		if image, ok := space.Opaque.Map["image"]; ok {
			metadata[xattrs.SpaceImageAttr] = string(image.Value)
		}
		if readme, ok := space.Opaque.Map["readme"]; ok {
			metadata[xattrs.SpaceReadmeAttr] = string(readme.Value)
		}
	}

	// TODO change the permission handling
	// these two attributes need manager permissions
	if space.Name != "" || hasDescription {
		err = fs.checkManagerPermission(ctx, node)
	}
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_PERMISSION_DENIED, Message: err.Error()},
		}, nil
	}
	// all other attributes need editor permissions
	err = fs.checkEditorPermission(ctx, node)
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_PERMISSION_DENIED, Message: err.Error()},
		}, nil
	}

	err = xattrs.SetMultiple(node.InternalPath(), metadata)
	if err != nil {
		return nil, err
	}

	// send back the updated data from the storage
	updatedSpace, err := fs.storageSpaceFromNode(ctx, node, "*", node.InternalPath(), false)
	if err != nil {
		return nil, err
	}

	return &provider.UpdateStorageSpaceResponse{
		Status:       &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		StorageSpace: updatedSpace,
	}, nil
}

// DeleteStorageSpace deletes a storage space
func (fs *Decomposedfs) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) error {
	opaque := req.Opaque
	var purge bool
	if opaque != nil {
		_, purge = opaque.Map["purge"]
	}

	spaceID := req.Id.OpaqueId

	matches, err := filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceTypeAny, spaceID))
	if err != nil {
		return err
	}

	if len(matches) != 1 {
		return fmt.Errorf("delete space failed: found %d matching spaces", len(matches))
	}

	target, err := os.Readlink(matches[0])
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("match", matches[0]).Msg("could not read link, skipping")
	}

	n, err := node.ReadNode(ctx, fs.lu, filepath.Base(target))
	if err != nil {
		return err
	}

	if purge {
		if !strings.Contains(n.Name, node.TrashIDDelimiter) {
			return errtypes.NewErrtypeFromStatus(status.NewInvalidArg(ctx, "can't purge enabled space"))
		}
		ip := fs.lu.InternalPath(req.Id.OpaqueId)
		matches, err := filepath.Glob(ip)
		if err != nil {
			return err
		}

		// TODO: remove blobs
		if err := os.RemoveAll(matches[0]); err != nil {
			return err
		}

		matches, err = filepath.Glob(filepath.Join(fs.o.Root, "spaces", spaceTypeAny, req.Id.OpaqueId))
		if err != nil {
			return err
		}
		if len(matches) != 1 {
			return fmt.Errorf("delete space failed: found %d matching spaces", len(matches))
		}

		if err := os.RemoveAll(matches[0]); err != nil {
			return err
		}

		matches, err = filepath.Glob(filepath.Join(fs.o.Root, "nodes", node.RootID, req.Id.OpaqueId+node.TrashIDDelimiter+"*"))
		if err != nil {
			return err
		}

		if len(matches) != 1 {
			return fmt.Errorf("delete root node failed: found %d matching root nodes", len(matches))
		}

		return os.RemoveAll(matches[0])
	}
	// don't delete - just rename
	dn := *n
	deletionTime := time.Now().UTC().Format(time.RFC3339Nano)
	dn.Name = n.Name + node.TrashIDDelimiter + deletionTime
	dn.Exists = false
	err = fs.tp.Move(ctx, n, &dn)
	if err != nil {
		return err
	}

	err = os.RemoveAll(matches[0])
	if err != nil {
		return err
	}

	trashPath := dn.InternalPath()
	np := filepath.Join(filepath.Dir(matches[0]), filepath.Base(trashPath))
	return os.Symlink(trashPath, np)
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
			// FIXME: is it ok to wipe this err if the symlink already exists?
			err = nil
		} else {
			// TODO how should we handle error cases here?
			appctx.GetLogger(ctx).Error().Err(err).Str("space", spaceID).Str("spacetype", spaceType).Msg("could not create symlink")
		}
	}

	return err
}

func (fs *Decomposedfs) storageSpaceFromNode(ctx context.Context, n *node.Node, spaceType, nodePath string, canListAllSpaces bool) (*provider.StorageSpace, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	if !canListAllSpaces {
		ok, err := node.NewPermissions(fs.lu).HasPermission(ctx, n, func(p *provider.ResourcePermissions) bool {
			return p.Stat
		})
		if err != nil || !ok {
			return nil, errtypes.PermissionDenied(fmt.Sprintf("user %s is not allowed to Stat the space %s", user.Username, n.ID))
		}

		if strings.Contains(n.Name, node.TrashIDDelimiter) {
			ok, err := node.NewPermissions(fs.lu).HasPermission(ctx, n, func(p *provider.ResourcePermissions) bool {
				// TODO: Which permission do I need to see the space?
				return p.AddGrant
			})
			if err != nil || !ok {
				return nil, errtypes.PermissionDenied(fmt.Sprintf("user %s is not allowed to list deleted spaces %s", user.Username, n.ID))
			}
		}
	}

	owner, err := n.Owner()
	if err != nil {
		return nil, err
	}

	// TODO apply more filters
	var sname string
	if sname, err = n.GetMetadata(xattrs.SpaceNameAttr); err != nil {
		// FIXME: Is that a severe problem?
		appctx.GetLogger(ctx).Debug().Err(err).Msg("space does not have a name attribute")
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

	grants, err := n.ListGrants(ctx)
	if err != nil {
		return nil, err
	}

	m := make(map[string]*provider.ResourcePermissions, len(grants))
	for _, g := range grants {
		var id string
		switch g.Grantee.Type {
		case provider.GranteeType_GRANTEE_TYPE_GROUP:
			id = g.Grantee.GetGroupId().OpaqueId
		case provider.GranteeType_GRANTEE_TYPE_USER:
			id = g.Grantee.GetUserId().OpaqueId
		default:
			continue
		}

		m[id] = g.Permissions
	}
	marshalled, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	space := &provider.StorageSpace{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"grants": {
					Decoder: "json",
					Value:   marshalled,
				},
			},
		},
		Id: &provider.StorageSpaceId{OpaqueId: n.SpaceRoot.ID},
		Root: &provider.ResourceId{
			StorageId: n.SpaceRoot.ID,
			OpaqueId:  n.SpaceRoot.ID,
		},
		Name:      sname,
		SpaceType: spaceType,
		// Mtime is set either as node.tmtime or as fi.mtime below
	}

	if strings.Contains(n.Name, node.TrashIDDelimiter) {
		space.Opaque.Map["trashed"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte("trashed"),
		}
	}

	if spaceType != spaceTypeProject && owner.OpaqueId != "" {
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
	} else if fi, err := os.Stat(nodePath); err == nil {
		// fall back to stat mtime
		un := fi.ModTime().UnixNano()
		space.Mtime = &types.Timestamp{
			Seconds: uint64(un / 1000000000),
			Nanos:   uint32(un % 1000000000),
		}
	}

	spaceAttributes, err := xattrs.All(nodePath)
	if err != nil {
		return nil, err
	}

	// quota
	quotaAttr, ok := spaceAttributes[xattrs.QuotaAttr]
	if ok {
		// make sure we have a proper signed int
		// we use the same magic numbers to indicate:
		// -1 = uncalculated
		// -2 = unknown
		// -3 = unlimited
		if quota, err := strconv.ParseUint(quotaAttr, 10, 64); err == nil {
			space.Quota = &provider.Quota{
				QuotaMaxBytes: quota,
				QuotaMaxFiles: math.MaxUint64, // TODO MaxUInt64? = unlimited? why even max files? 0 = unlimited?
			}
		} else {
			return nil, err
		}
	}
	spaceImage, ok := spaceAttributes[xattrs.SpaceImageAttr]
	if ok {
		space.Opaque.Map["image"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(spaceImage),
		}
	}
	spaceDescription, ok := spaceAttributes[xattrs.SpaceDescriptionAttr]
	if ok {
		space.Opaque.Map["description"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(spaceDescription),
		}
	}
	spaceReadme, ok := spaceAttributes[xattrs.SpaceReadmeAttr]
	if ok {
		space.Opaque.Map["readme"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(spaceReadme),
		}
	}
	return space, nil
}

func (fs *Decomposedfs) checkManagerPermission(ctx context.Context, n *node.Node) error {
	// to update the space name or short description we need the manager role
	// current workaround: check if AddGrant Permission exists
	managerPerm, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		return rp.AddGrant
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !managerPerm:
		msg := fmt.Sprintf("not enough permissions to change attributes on %s", filepath.Join(n.ParentID, n.Name))
		return errtypes.PermissionDenied(msg)
	}
	return nil
}

func (fs *Decomposedfs) checkEditorPermission(ctx context.Context, n *node.Node) error {
	// current workaround: check if InitiateFileUpload Permission exists
	editorPerm, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !editorPerm:
		msg := fmt.Sprintf("not enough permissions to change attributes on %s", filepath.Join(n.ParentID, n.Name))
		return errtypes.PermissionDenied(msg)
	}
	return nil
}
