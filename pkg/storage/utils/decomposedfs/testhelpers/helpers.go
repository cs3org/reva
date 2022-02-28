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

	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/google/uuid"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/tests/helpers"
)

// TestEnv represents a test environment for unit tests
type TestEnv struct {
	Root              string
	Fs                storage.FS
	Tree              *tree.Tree
	Permissions       *mocks.PermissionsChecker
	Blobstore         *treemocks.Blobstore
	Owner             *userpb.User
	Lookup            *lookup.Lookup
	Ctx               context.Context
	SpaceRootRes      *providerv1beta1.ResourceId
	PermissionsClient *mocks.CS3PermissionsClient
}

// NewTestEnv prepares a test environment on disk
// The storage contains some directories and a file:
//
//  /dir1/
//  /dir1/file1
//  /dir1/subdir1/
func NewTestEnv() (*TestEnv, error) {
	tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
	if err != nil {
		return nil, err
	}

	config := map[string]interface{}{
		"root":                tmpRoot,
		"treetime_accounting": true,
		"treesize_accounting": true,
		"share_folder":        "/Shares",
		"user_layout":         "{{.Id.OpaqueId}}",
	}
	o, err := options.New(config)
	if err != nil {
		return nil, err
	}

	owner := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: "u-s-e-r-id",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
		Username: "username",
	}
	lookup := &lookup.Lookup{Options: o}
	permissions := &mocks.PermissionsChecker{}
	cs3permissionsclient := &mocks.CS3PermissionsClient{}
	bs := &treemocks.Blobstore{}
	tree := tree.New(o.Root, true, true, lookup, bs)
	fs, err := decomposedfs.New(o, lookup, permissions, tree, cs3permissionsclient)
	if err != nil {
		return nil, err
	}
	ctx := ruser.ContextSetUser(context.Background(), owner)

	env := &TestEnv{
		Root:              tmpRoot,
		Fs:                fs,
		Tree:              tree,
		Lookup:            lookup,
		Permissions:       permissions,
		Blobstore:         bs,
		Owner:             owner,
		Ctx:               ctx,
		PermissionsClient: cs3permissionsclient,
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
	err = n.WriteAllNodeMetadata()
	if err != nil {
		return nil, err
	}
	// Link in parent
	childNameLink := filepath.Join(n.ParentInternalPath(), n.Name)
	err = os.Symlink("../../../../../"+lookup.Pathify(n.ID, 4, 2), childNameLink)
	if err != nil {
		return nil, err
	}

	return n, n.FindStorageSpaceRoot()
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
	space, err := t.Fs.CreateStorageSpace(t.Ctx, &providerv1beta1.CreateStorageSpaceRequest{
		Owner: t.Owner,
		Type:  typ,
		Quota: quota,
	})
	if err != nil {
		return nil, err
	}

	ref := buildRef(space.StorageSpace.Id.OpaqueId, "")

	// the space name attribute is the stop condition in the lookup
	h, err := node.ReadNode(t.Ctx, t.Lookup, space.StorageSpace.Id.OpaqueId, space.StorageSpace.Id.OpaqueId)
	if err != nil {
		return nil, err
	}
	if err = xattr.Set(h.InternalPath(), xattrs.SpaceNameAttr, []byte("username")); err != nil {
		return nil, err
	}

	// Create dir1
	t.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(1) // Permissions required for setup below
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
	t.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(1) // Permissions required for setup below
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
	t.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(1) // Permissions required for setup below
	ref.Path = "/emptydir"
	err = t.Fs.CreateDir(t.Ctx, ref)
	if err != nil {
		return nil, err
	}

	return ref.ResourceId, nil
}

// shortcut to get a ref
func buildRef(id, path string) *providerv1beta1.Reference {
	return &providerv1beta1.Reference{
		ResourceId: &providerv1beta1.ResourceId{
			StorageId: id,
			OpaqueId:  id,
		},
		Path: path,
	}
}
