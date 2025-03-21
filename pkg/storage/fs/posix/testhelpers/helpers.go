// Copyright 2018-2024 CERN
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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	"maps"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/cache"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/lookup"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/options"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/timemanager"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/trashbin"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/tree"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/aspects"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/prefixes"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	nodemocks "github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node/mocks"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/permissions"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/permissions/mocks"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/usermapper"
	"github.com/opencloud-eu/reva/v2/pkg/storagespace"
	"github.com/opencloud-eu/reva/v2/pkg/store"
	"github.com/opencloud-eu/reva/v2/tests/helpers"
)

// TestEnv represents a test environment for unit tests
type TestEnv struct {
	Root                 string
	Fs                   *decomposedfs.Decomposedfs
	Tree                 *tree.Tree
	Permissions          *mocks.PermissionsChecker
	Blobstore            *nodemocks.Blobstore
	Owner                *userpb.User
	DeleteAllSpacesUser  *userpb.User
	DeleteHomeSpacesUser *userpb.User
	Users                []*userpb.User
	Lookup               *lookup.Lookup
	Ctx                  context.Context
	SpaceRootRes         *providerv1beta1.ResourceId
	PermissionsClient    *mocks.CS3PermissionsClient
	Options              *options.Options
}

func (e *TestEnv) GetFs() storage.FS {
	return e.Fs
}

func (e *TestEnv) GetPermissions() *mocks.PermissionsChecker {
	return e.Permissions
}

func (e *TestEnv) GetCtx() context.Context {
	return e.Ctx
}

func (e *TestEnv) GetSpaceRootRes() *providerv1beta1.ResourceId {
	return e.SpaceRootRes
}

func (e *TestEnv) GetBlobstore() *nodemocks.Blobstore {
	return e.Blobstore
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
	um := &usermapper.NullMapper{}

	tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
	if err != nil {
		return nil, err
	}
	defaultConfig := map[string]interface{}{
		"root":                       tmpRoot,
		"treetime_accounting":        true,
		"treesize_accounting":        true,
		"personalspacepath_template": "users/{{.User.Username}}",
		"generalspacepath_template":  "projects/{{.SpaceId}}",
		"watch_fs":                   false,
		"scan_fs":                    true,
	}
	// make it possible to override single config values
	maps.Copy(defaultConfig, config)

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

	var lu *lookup.Lookup
	switch o.MetadataBackend {
	case "xattrs":
		lu = lookup.New(metadata.NewXattrsBackend(o.FileMetadataCache), um, o, &timemanager.Manager{})
	case "hybrid":
		lu = lookup.New(metadata.NewHybridBackend(1024,
			func(n metadata.MetadataNode) string {
				spaceRoot, _ := lu.IDCache.Get(context.Background(), n.GetSpaceID(), n.GetSpaceID())
				if len(spaceRoot) == 0 {
					return ""
				}

				return filepath.Join(spaceRoot, lookup.MetadataDir)
			},
			cache.Config{
				Database: o.Root,
			}), um, o, &timemanager.Manager{})
	case "messagepack":
		lu = lookup.New(metadata.NewMessagePackBackend(o.FileMetadataCache), um, o, &timemanager.Manager{})
	default:
		return nil, fmt.Errorf("unknown metadata backend %s", o.MetadataBackend)
	}

	pmock := &mocks.PermissionsChecker{}

	cs3permissionsclient := &mocks.CS3PermissionsClient{}
	pool.RemoveSelector("PermissionsSelector" + "any")
	permissionsSelector := pool.GetSelector[cs3permissions.PermissionsAPIClient](
		"PermissionsSelector",
		"any",
		func(cc grpc.ClientConnInterface) cs3permissions.PermissionsAPIClient {
			return cs3permissionsclient
		},
	)

	logger := zerolog.New(os.Stderr).With().Logger()

	bs := &nodemocks.Blobstore{}
	p := permissions.NewPermissions(pmock, permissionsSelector)
	tb, err := trashbin.New(o, p, lu, &logger)
	if err != nil {
		return nil, err
	}
	tree, err := tree.New(lu, bs, um, tb, permissions.Permissions{}, o, nil, store.Create(), &logger)
	if err != nil {
		return nil, err
	}
	aspects := aspects.Aspects{
		Lookup:      lu,
		Tree:        tree,
		Permissions: p,
		Trashbin:    tb,
	}
	fs, err := decomposedfs.New(&o.Options, aspects, &logger)
	if err != nil {
		return nil, err
	}
	err = tb.Setup(fs)
	if err != nil {
		return nil, err
	}
	ctx := ruser.ContextSetUser(context.Background(), owner)

	tmpFs, _ := fs.(*decomposedfs.Decomposedfs)

	env := &TestEnv{
		Root:                 tmpRoot,
		Fs:                   tmpFs,
		Tree:                 tree,
		Lookup:               lu,
		Permissions:          pmock,
		Blobstore:            bs,
		Owner:                owner,
		DeleteAllSpacesUser:  deleteAllSpacesUser,
		DeleteHomeSpacesUser: deleteHomeSpacesUser,
		Users:                users,
		Ctx:                  ctx,
		PermissionsClient:    cs3permissionsclient,
		Options:              o,
	}

	env.SpaceRootRes, err = env.CreateTestStorageSpace("personal", nil)
	return env, err
}

// Cleanup removes all files from disk
func (t *TestEnv) Cleanup() {
	for range 5 {
		err := os.RemoveAll(t.Root)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
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
	nodePath := filepath.Join(n.ParentPath(), n.Name)
	if err := os.MkdirAll(filepath.Dir(nodePath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(nodePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, blobSize)
	if _, err := f.Write(buf); err != nil {
		return nil, err
	}

	if err := f.Close(); err != nil {
		return nil, err
	}

	err = t.Lookup.CacheID(t.Ctx, spaceID, n.ID, nodePath)
	if err != nil {
		return nil, err
	}
	attrs := n.NodeMetadata(t.Ctx)
	attrs.SetString(prefixes.IDAttr, n.ID)
	err = n.SetXattrs(attrs, true)
	if err != nil {
		return nil, err
	}
	if err := n.FindStorageSpaceRoot(t.Ctx); err != nil {
		return nil, err
	}

	return n, t.Tree.Propagate(t.Ctx, n, blobSize)

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
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&providerv1beta1.ResourcePermissions{
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
	if err = h.SetXattr(t.Ctx, prefixes.SpaceNameAttr, []byte("username")); err != nil {
		return nil, err
	}

	// Create dir1
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&providerv1beta1.ResourcePermissions{
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
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&providerv1beta1.ResourcePermissions{
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
	t.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&providerv1beta1.ResourcePermissions{
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
