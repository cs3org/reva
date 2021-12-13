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

	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/google/uuid"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
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
	Root         string
	Fs           storage.FS
	Tree         *tree.Tree
	Permissions  *mocks.PermissionsChecker
	Blobstore    *treemocks.Blobstore
	Owner        *userpb.User
	Lookup       *decomposedfs.Lookup
	Ctx          context.Context
	SpaceRootRes *providerv1beta1.ResourceId
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
			OpaqueId: "userid",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
		Username: "username",
	}
	lookup := &decomposedfs.Lookup{Options: o}
	permissions := &mocks.PermissionsChecker{}
	permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(4) // Permissions required for setup below
	bs := &treemocks.Blobstore{}
	tree := tree.New(o.Root, true, true, lookup, bs)
	fs, err := decomposedfs.New(o, lookup, permissions, tree)
	if err != nil {
		return nil, err
	}
	ctx := ruser.ContextSetUser(context.Background(), owner)

	home, err := fs.CreateStorageSpace(ctx, &providerv1beta1.CreateStorageSpaceRequest{
		Owner: owner,
		Type:  "personal",
	})
	if err != nil {
		return nil, err
	}

	env := &TestEnv{
		Root:         tmpRoot,
		Fs:           fs,
		Tree:         tree,
		Lookup:       lookup,
		Permissions:  permissions,
		Blobstore:    bs,
		Owner:        owner,
		Ctx:          ctx,
		SpaceRootRes: home.StorageSpace.Root,
	}

	spaceRootRef := &providerv1beta1.Reference{
		ResourceId: env.SpaceRootRes,
	}

	// the space name attribute is the stop condition in the lookup
	h, err := node.ReadNode(ctx, lookup, home.StorageSpace.Id.OpaqueId)
	if err != nil {
		return nil, err
	}
	if err = xattr.Set(h.InternalPath(), xattrs.SpaceNameAttr, []byte("username")); err != nil {
		return nil, err
	}

	// Create dir1
	dir1, err := env.CreateTestDir("./dir1", spaceRootRef)
	if err != nil {
		return nil, err
	}

	// Create file1 in dir1
	_, err = env.CreateTestFile(home.StorageSpace.Root.StorageId, "file1", "file1-blobid", 1234, dir1.ID)
	if err != nil {
		return nil, err
	}

	// Create subdir1 in dir1
	spaceRootRef.Path = "./dir1/subdir1"
	err = fs.CreateDir(ctx, spaceRootRef)
	if err != nil {
		return nil, err
	}

	dir2, err := dir1.Child(ctx, "subdir1, spaceRootRef")
	if err != nil {
		return nil, err
	}

	// Create file1 in dir1
	_, err = env.CreateTestFile(home.StorageSpace.Root.StorageId, "file2", "file2-blobid", 12345, dir2.ID)
	if err != nil {
		return nil, err
	}

	// Create emptydir
	spaceRootRef.Path = "/emptydir"
	err = fs.CreateDir(ctx, spaceRootRef)
	if err != nil {
		return nil, err
	}

	return env, nil
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
func (t *TestEnv) CreateTestFile(spaceRoot, name, blobID string, blobSize int64, parentID string) (*node.Node, error) {
	// Create file in dir1
	file := node.New(
		spaceRoot,
		uuid.New().String(),
		parentID,
		name,
		blobSize,
		blobID,
		nil,
		t.Lookup,
	)
	_, err := os.OpenFile(file.InternalPath(), os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	err = file.WriteMetadata(t.Owner.Id)
	if err != nil {
		return nil, err
	}
	// Link in parent
	childNameLink := filepath.Join(t.Lookup.InternalPath(file.ParentID), file.Name)
	err = os.Symlink("../"+file.ID, childNameLink)
	if err != nil {
		return nil, err
	}

	return file, err
}
