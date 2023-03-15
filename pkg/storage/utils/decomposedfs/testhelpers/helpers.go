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

package helpers

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/v2/tests/helpers"
)

// TestEnv represents a test environment for unit tests
type TestEnv struct {
	Root                 string
	Fs                   *decomposedfs.Decomposedfs
	Tree                 *tree.Tree
	Permissions          *mocks.PermissionsChecker
	Blobstore            *treemocks.Blobstore
	Owner                *userpb.User
	DeleteAllSpacesUser  *userpb.User
	DeleteHomeSpacesUser *userpb.User
	Users                []*userpb.User
	Lookup               *lookup.Lookup
	Ctx                  context.Context
	SpaceRootRes         *providerv1beta1.ResourceId
	PermissionsClient    *mocks.CS3PermissionsClient
}

// Constant UUIDs for the space users
const (
	OwnerID                = "25b69780-5f39-43be-a7ac-a9b9e9fe4230"
	DeleteAllSpacesUserID  = "39885dbc-68c0-47c0-a873-9d5e5646dceb"
	DeleteHomeSpacesUserID = "ca8c6bf1-36a7-4d10-87a5-a2806566f983"
	User0ID                = "824385ae-8fc6-4896-8eb2-d1d171290bd0"
	User1ID                = "693b0d96-80a2-4016-b53d-425ce4f66114"
)

// NewTestEnv prepares a test environment on disk
// The storage contains some directories and a file:
//
//	/dir1/
//	/dir1/file1
//	/dir1/subdir1/
//
// The default config can be overridden by providing the strings to override
// via map as a parameter
func NewTestEnv(config map[string]interface{}) (*TestEnv, error) {
	tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
	if err != nil {
		return nil, err
	}
	defaultConfig := map[string]interface{}{
		"root":                tmpRoot,
		"treetime_accounting": true,
		"treesize_accounting": true,
		"share_folder":        "/Shares",
		"user_layout":         "{{.Id.OpaqueId}}",
	}
	// make it possible to override single config values
	for k, v := range config {
		defaultConfig[k] = v
	}

	o, err := options.New(defaultConfig)
	if err != nil {
		return nil, err
	}

	owner := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: OwnerID,
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
		Username: "username",
	}
	deleteHomeSpacesUser := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: DeleteHomeSpacesUserID,
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
		Username: "username",
	}
	deleteAllSpacesUser := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: DeleteAllSpacesUserID,
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
		Username: "username",
	}
	users := []*userpb.User{
		{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: User0ID,
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
		},
		{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: User1ID,
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
		},
	}
	lookup := lookup.New(metadata.XattrsBackend{}, o)
	permissions := &mocks.PermissionsChecker{}
	cs3permissionsclient := &mocks.CS3PermissionsClient{}
	bs := &treemocks.Blobstore{}
	tree := tree.New(o.Root, true, true, lookup, bs)
	fs, err := decomposedfs.New(o, lookup, decomposedfs.NewPermissions(permissions, cs3permissionsclient), tree, nil)
	if err != nil {
		return nil, err
	}
	ctx := ruser.ContextSetUser(context.Background(), owner)

	tmpFs, _ := fs.(*decomposedfs.Decomposedfs)

	env := &TestEnv{
		Root:                 tmpRoot,
		Fs:                   tmpFs,
		Tree:                 tree,
		Lookup:               lookup,
		Permissions:          permissions,
		Blobstore:            bs,
		Owner:                owner,
		DeleteAllSpacesUser:  deleteAllSpacesUser,
		DeleteHomeSpacesUser: deleteHomeSpacesUser,
		Users:                users,
		Ctx:                  ctx,
		PermissionsClient:    cs3permissionsclient,
	}

	env.SpaceRootRes, err = env.CreateTestStorageSpace("personal", nil)
	return env, err
}

// Cleanup removes all files from disk
func (t *TestEnv) Cleanup() {
	os.RemoveAll(t.Root)
}

// CreateTestDir create a directory and returns a corresponding Node
func (t *TestEnv) CreateTestDir(name string, parentRef *providerv1beta1.Reference) (*node.Node, error) {
	ref := parentRef
	ref.Path = name

	err := t.Fs.CreateDir(t.Ctx, ref)
	if err != nil {
		return nil, err
	}

	ref.Path = name
	n, err := t.Lookup.NodeFromResource(t.Ctx, ref)
	if err != nil {
		return nil, err
	}

	return n, nil
}

// CreateTestFile creates a new file and its metadata and returns a corresponding Node
func (t *TestEnv) CreateTestFile(name, blobID, parentID, spaceID string, blobSize int64) (*node.Node, error) {
	// Create n in dir1
	n := node.New(
		spaceID,
		uuid.New().String(),
		parentID,
		name,
		blobSize,
		blobID,
		providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
		nil,
		t.Lookup,
	)
	nodePath := n.InternalPath()
	if err := os.MkdirAll(filepath.Dir(nodePath), 0700); err != nil {
		return nil, err
	}
	_, err := os.OpenFile(nodePath, os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	err = n.WriteAllNodeMetadata(context.Background())
	if err != nil {
		return nil, err
	}
	// Link in parent
	childNameLink := filepath.Join(n.ParentPath(), n.Name)
	err = os.Symlink("../../../../../"+lookup.Pathify(n.ID, 4, 2), childNameLink)
	if err != nil {
		return nil, err
	}
	if err := n.FindStorageSpaceRoot(); err != nil {
		return nil, err
	}

	return n, t.Tree.Propagate(context.Background(), n, blobSize)

}

// CreateTestStorageSpace will create a storage space with some directories and files
// It returns the ResourceId of the space
//
// /dir1/
// /dir1/file1
// /dir1/subdir1
func (t *TestEnv) CreateTestStorageSpace(typ string, quota *providerv1beta1.Quota) (*providerv1beta1.ResourceId, error) {
	t.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Times(1).Return(&cs3permissions.CheckPermissionResponse{
		Status: &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
	}, nil)
	// Permissions required for setup below
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(providerv1beta1.ResourcePermissions{
		Stat:     true,
		AddGrant: true,
	}, nil).Times(1) //

	var owner *userpb.User
	if typ == "personal" {
		owner = t.Owner
	}
	space, err := t.Fs.CreateStorageSpace(t.Ctx, &providerv1beta1.CreateStorageSpaceRequest{
		Owner: owner,
		Type:  typ,
		Quota: quota,
	})
	if err != nil {
		return nil, err
	}

	ref := buildRef(space.StorageSpace.Id.OpaqueId, "")

	// the space name attribute is the stop condition in the lookup
	sid, err := storagespace.ParseID(space.StorageSpace.Id.OpaqueId)
	if err != nil {
		return nil, err
	}
	h, err := node.ReadNode(t.Ctx, t.Lookup, sid.SpaceId, sid.OpaqueId, false, nil, false)
	if err != nil {
		return nil, err
	}
	if err = h.SetXattr(prefixes.SpaceNameAttr, []byte("username")); err != nil {
		return nil, err
	}

	// Create dir1
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(providerv1beta1.ResourcePermissions{
		Stat:            true,
		CreateContainer: true,
	}, nil).Times(1) // Permissions required for setup below
	dir1, err := t.CreateTestDir("./dir1", ref)
	if err != nil {
		return nil, err
	}

	// Create file1 in dir1
	_, err = t.CreateTestFile("file1", "file1-blobid", dir1.ID, dir1.SpaceID, 1234)
	if err != nil {
		return nil, err
	}

	// Create subdir1 in dir1
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(providerv1beta1.ResourcePermissions{
		Stat:            true,
		CreateContainer: true,
	}, nil).Times(1) // Permissions required for setup below
	ref.Path = "./dir1/subdir1"
	err = t.Fs.CreateDir(t.Ctx, ref)
	if err != nil {
		return nil, err
	}

	_, err = dir1.Child(t.Ctx, "subdir1, ref")
	if err != nil {
		return nil, err
	}

	// Create emptydir
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(providerv1beta1.ResourcePermissions{
		Stat:            true,
		CreateContainer: true,
	}, nil).Times(1) // Permissions required for setup below
	ref.Path = "/emptydir"
	err = t.Fs.CreateDir(t.Ctx, ref)
	if err != nil {
		return nil, err
	}

	return ref.ResourceId, nil
}

// shortcut to get a ref
func buildRef(id, path string) *providerv1beta1.Reference {
	res, err := storagespace.ParseID(id)
	if err != nil {
		return nil
	}
	return &providerv1beta1.Reference{
		ResourceId: &providerv1beta1.ResourceId{
			StorageId: res.StorageId,
			SpaceId:   res.SpaceId,
			OpaqueId:  res.OpaqueId,
		},
		Path: path,
	}
}
