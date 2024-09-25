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

package decomposedfs_test

import (
	"bytes"
	"context"
	"io"
	"os"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/cache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/aspects"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/permissions"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/permissions/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/timemanager"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/store"
	"github.com/cs3org/reva/v2/tests/helpers"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("File uploads", func() {
	var (
		ref     *provider.Reference
		rootRef *provider.Reference
		fs      storage.FS
		user    *userpb.User
		ctx     context.Context

		o                    *options.Options
		lu                   *lookup.Lookup
		pmock                *mocks.PermissionsChecker
		cs3permissionsclient *mocks.CS3PermissionsClient
		permissionsSelector  pool.Selectable[cs3permissions.PermissionsAPIClient]
		bs                   *treemocks.Blobstore
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId: "u-s-e-r-id",
			},
			Path: "/foo",
		}

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "u-s-e-r-id",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "username",
		}

		rootRef = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId:  "u-s-e-r-id",
				OpaqueId: "u-s-e-r-id",
			},
			Path: "/",
		}

		ctx = ruser.ContextSetUser(context.Background(), user)

		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		o, err = options.New(map[string]interface{}{
			"root": tmpRoot,
		})
		Expect(err).ToNot(HaveOccurred())
		lu = lookup.New(metadata.NewXattrsBackend(o.Root, cache.Config{}), o, &timemanager.Manager{})
		pmock = &mocks.PermissionsChecker{}
		cs3permissionsclient = &mocks.CS3PermissionsClient{}
		pool.RemoveSelector("PermissionsSelector" + "any")
		permissionsSelector = pool.GetSelector[cs3permissions.PermissionsAPIClient](
			"PermissionsSelector",
			"any",
			func(cc grpc.ClientConnInterface) cs3permissions.PermissionsAPIClient {
				return cs3permissionsclient
			},
		)

		bs = &treemocks.Blobstore{}
	})

	AfterEach(func() {
		root := o.Root
		if root != "" {
			os.RemoveAll(root)
		}
	})

	BeforeEach(func() {
		cs3permissionsclient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&cs3permissions.CheckPermissionResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		}, nil)
		pmock.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
			Stat:     true,
			AddGrant: true,
		}, nil).Times(1)
		var err error
		bs.On("GetAvailableSize", mock.Anything).Return(uint64(1000000000), nil).Times(1)
		tree := tree.New(lu, bs, o, store.Create())

		aspects := aspects.Aspects{
			Lookup:      lu,
			Tree:        tree,
			Blobstore:   bs,
			Permissions: permissions.NewPermissions(pmock, permissionsSelector),
			Trashbin:    &decomposedfs.DecomposedfsTrashbin{},
		}
		fs, err = decomposedfs.New(o, aspects)
		Expect(err).ToNot(HaveOccurred())

		resp, err := fs.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{Owner: user, Type: "personal"})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Status.Code).To(Equal(v1beta11.Code_CODE_OK))
		resID, err := storagespace.ParseID(resp.StorageSpace.Id.OpaqueId)
		Expect(err).ToNot(HaveOccurred())
		ref.ResourceId = &resID
	})

	Context("the user's quota is exceeded", func() {
		BeforeEach(func() {
			pmock.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
				Stat:     true,
				GetQuota: true,
			}, nil)
		})
		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				var originalFunc = node.CheckQuota
				node.CheckQuota = func(ctx context.Context, spaceRoot *node.Node, overwrite bool, oldSize, newSize uint64) (quotaSufficient bool, err error) {
					return false, errtypes.InsufficientStorage("quota exceeded")
				}
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError(errtypes.InsufficientStorage("quota exceeded")))
				node.CheckQuota = originalFunc
			})
		})
	})

	Context("the user has insufficient permissions", func() {
		BeforeEach(func() {
			pmock.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
				Stat: true,
			}, nil)
		})

		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				msg := "error: permission denied: u-s-e-r-id!u-s-e-r-id/foo"
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError(msg))
			})
		})
	})

	Context("with insufficient permissions, home node", func() {
		JustBeforeEach(func() {
			var err error
			// the space name attribute is the stop condition in the lookup
			h, err := lu.NodeFromResource(ctx, rootRef)
			Expect(err).ToNot(HaveOccurred())
			err = h.SetXattrString(ctx, prefixes.SpaceNameAttr, "username")
			Expect(err).ToNot(HaveOccurred())
			pmock.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
				Stat: true,
			}, nil)
		})

		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				msg := "error: permission denied: u-s-e-r-id!u-s-e-r-id/foo"
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError(msg))
			})
		})
	})

	Context("with sufficient permissions", func() {
		BeforeEach(func() {
			pmock.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					Stat:               true,
					GetQuota:           true,
					InitiateFileUpload: true,
					ListContainer:      true,
				}, nil)
		})

		When("the user initiates a non zero byte file upload", func() {
			It("succeeds", func() {
				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(0))
			})
		})

		When("the user initiates a zero byte file upload", func() {
			It("succeeds", func() {
				bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("string"), mock.Anything).
					Return(nil)
				uploadIds, err := fs.InitiateUpload(ctx, ref, 0, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(1))
			})

			It("fails when trying to upload empty data. 0-byte uploads are finished during initialization already", func() {
				bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("string"), mock.Anything).
					Return(nil)
				uploadIds, err := fs.InitiateUpload(ctx, ref, 0, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())

				uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

				_, err = fs.Upload(ctx, storage.UploadRequest{
					Ref:    uploadRef,
					Body:   io.NopCloser(bytes.NewReader([]byte(""))),
					Length: 0,
				}, nil)

				Expect(err).To(HaveOccurred())
			})
		})

		When("the user uploads a non zero byte file", func() {
			It("succeeds", func() {
				var (
					fileContent = []byte("0123456789")
				)

				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

				bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("string"), mock.Anything).
					Return(nil).
					Run(func(args mock.Arguments) {
						data, err := os.ReadFile(args.Get(1).(string))

						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal([]byte("0123456789")))
					})

				_, err = fs.Upload(ctx, storage.UploadRequest{
					Ref:    uploadRef,
					Body:   io.NopCloser(bytes.NewReader(fileContent)),
					Length: int64(len(fileContent)),
				}, nil)

				Expect(err).ToNot(HaveOccurred())
				bs.AssertCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

				resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(1))
				Expect(resources[0].Path).To(Equal(ref.Path))
			})
		})

		When("the user tries to upload a file without intialising the upload", func() {
			It("fails", func() {
				var (
					fileContent = []byte("0123456789")
				)

				uploadRef := &provider.Reference{Path: "/some-non-existent-upload-reference"}
				_, err := fs.Upload(ctx, storage.UploadRequest{
					Ref:    uploadRef,
					Body:   io.NopCloser(bytes.NewReader(fileContent)),
					Length: int64(len(fileContent)),
				}, nil)

				Expect(err).To(HaveOccurred())

				resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(0))
			})
		})

	})
})
