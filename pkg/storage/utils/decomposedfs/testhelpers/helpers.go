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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	ruser "github.com/cs3org/reva/pkg/user"
)

// TestEnv represents a test environment for unit tests
type TestEnv struct {
	Root        string
	Fs          storage.FS
	Tree        *tree.Tree
	Permissions *mocks.PermissionsChecker
	Blobstore   *treemocks.Blobstore
	Owner       *userpb.User
	Lookup      *decomposedfs.Lookup
	Ctx         context.Context
}

// Cleanup removes all files from disk
func (t *TestEnv) Cleanup() {
	os.RemoveAll(t.Root)
}

// NewTestEnv prepares a test environment on disk
// The storage contains some directories and a file:
//
//  dir1/
//  dir1/file1
//  dir1/subdir1/
func NewTestEnv() (*TestEnv, error) {
	tmpRoot, err := ioutil.TempDir("", "reva-unit-tests-*-root")
	if err != nil {
		return nil, err
	}

	config := map[string]interface{}{
		"root":         tmpRoot,
		"enable_home":  true,
		"share_folder": "/Shares",
		"user_layout":  "{{.Id.OpaqueId}}",
	}
	o, err := options.New(config)
	if err != nil {
		return nil, err
	}

	owner := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: "userid",
		},
		Username: "username",
	}
	lookup := &decomposedfs.Lookup{Options: o}
	permissions := &mocks.PermissionsChecker{}
	permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Twice() // Permissions required for setup below
	bs := &treemocks.Blobstore{}
	tree := tree.New(o.Root, true, true, lookup, bs)
	fs, err := decomposedfs.New(o, lookup, permissions, tree)
	if err != nil {
		return nil, err
	}
	ctx := ruser.ContextSetUser(context.Background(), owner)

	// Create home
	err = fs.CreateHome(ctx)
	if err != nil {
		return nil, err
	}

	// Create dir1
	err = fs.CreateDir(ctx, "dir1")
	if err != nil {
		return nil, err
	}
	dir1, err := lookup.NodeFromPath(ctx, "dir1")
	if err != nil {
		return nil, err
	}

	// Create subdir1 in dir1
	err = fs.CreateDir(ctx, "dir1/subdir1")
	if err != nil {
		return nil, err
	}

	// Create file in dir1
	file := node.New(
		uuid.New().String(),
		dir1.ID,
		"file1",
		1234,
		"file1-blobid",
		nil,
		lookup,
	)
	_, err = os.OpenFile(file.InternalPath(), os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}
	err = file.WriteMetadata(owner.Id)
	if err != nil {
		return nil, err
	}
	// Link in parent
	childNameLink := filepath.Join(lookup.InternalPath(file.ParentID), file.Name)
	err = os.Symlink("../"+file.ID, childNameLink)
	if err != nil {
		return nil, err
	}

	return &TestEnv{
		Root:        tmpRoot,
		Fs:          fs,
		Tree:        tree,
		Lookup:      lookup,
		Permissions: permissions,
		Blobstore:   bs,
		Owner:       owner,
		Ctx:         ctx,
	}, nil
}
