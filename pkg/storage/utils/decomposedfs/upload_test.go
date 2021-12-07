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
	"io/ioutil"
	"os"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/tests/helpers"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("File uploads", func() {
	var (
		ref     *provider.Reference
		rootRef *provider.Reference
		fs      storage.FS
		user    *userpb.User
		ctx     context.Context
		spaceID string

		o           *options.Options
		lookup      *decomposedfs.Lookup
		permissions *mocks.PermissionsChecker
		bs          *treemocks.Blobstore
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: "userid",
			},
			Path: "/foo",
		}

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "username",
		}

		rootRef = &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: "userid",
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
		lookup = &decomposedfs.Lookup{Options: o}
		permissions = &mocks.PermissionsChecker{}
		bs = &treemocks.Blobstore{}
	})

	AfterEach(func() {
		root := o.Root
		if root != "" {
			os.RemoveAll(root)
		}
	})

	BeforeEach(func() {
		permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Times(1).Return(true, nil)
		var err error
		tree := tree.New(o.Root, true, true, lookup, bs)
		fs, err = decomposedfs.New(o, lookup, permissions, tree)
		Expect(err).ToNot(HaveOccurred())

		resp, err := fs.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{Owner: user, Type: "personal"})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Status.Code).To(Equal(v1beta11.Code_CODE_OK))
		spaceID = resp.StorageSpace.Id.OpaqueId
		ref.ResourceId = &provider.ResourceId{StorageId: spaceID}
	})

	Context("the user's quota is exceeded", func() {
		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				var originalFunc = node.CheckQuota
				node.CheckQuota = func(spaceRoot *node.Node, fileSize uint64) (quotaSufficient bool, err error) {
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
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		})

		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				msg := "error: permission denied: userid/foo"
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError(msg))
			})
		})
	})

	Context("with insufficient permissions, home node", func() {
		JustBeforeEach(func() {
			var err error
			// the space name attribute is the stop condition in the lookup
			h, err := lookup.RootNode(ctx)
			Expect(err).ToNot(HaveOccurred())
			err = xattr.Set(h.InternalPath(), xattrs.SpaceNameAttr, []byte("username"))
			Expect(err).ToNot(HaveOccurred())
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		})

		When("the user wants to initiate a file upload", func() {
			It("fails", func() {
				msg := "error: permission denied: userid/foo"
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError(msg))
			})
		})
	})

	Context("with sufficient permissions", func() {
		BeforeEach(func() {
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
			permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(provider.ResourcePermissions{
					ListContainer: true,
				}, nil)
		})

		When("the user initiates a non zero byte file upload", func() {
			It("succeeds", func() {
				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				resources, err := fs.ListFolder(ctx, rootRef, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(1)) // .space folder
				Expect(resources[0].Path).To(Equal("/.space"))
			})
		})

		When("the user initiates a zero byte file upload", func() {
			It("succeeds", func() {
				uploadIds, err := fs.InitiateUpload(ctx, ref, 0, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				resources, err := fs.ListFolder(ctx, rootRef, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(1)) // .space folder
				Expect(resources[0].Path).To(Equal("/.space"))
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

				bs.On("Upload", mock.AnythingOfType("string"), mock.AnythingOfType("*os.File")).
					Return(nil).
					Run(func(args mock.Arguments) {
						reader := args.Get(1).(io.Reader)
						data, err := ioutil.ReadAll(reader)

						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal([]byte("0123456789")))
					})

				err = fs.Upload(ctx, uploadRef, ioutil.NopCloser(bytes.NewReader(fileContent)))

				Expect(err).ToNot(HaveOccurred())
				bs.AssertCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything)

				resources, err := fs.ListFolder(ctx, rootRef, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(2)) // .space folder & uploaded resource
				Expect(resources[0].Path).To(Or(Equal(ref.Path), Equal("/.space")))
				Expect(resources[1].Path).To(Or(Equal(ref.Path), Equal("/.space")))
			})
		})

		When("the user uploads a zero byte file", func() {
			It("succeeds", func() {
				var (
					fileContent = []byte("")
				)

				uploadIds, err := fs.InitiateUpload(ctx, ref, 0, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

				bs.On("Upload", mock.AnythingOfType("string"), mock.AnythingOfType("*os.File")).
					Return(nil).
					Run(func(args mock.Arguments) {
						reader := args.Get(1).(io.Reader)
						data, err := ioutil.ReadAll(reader)

						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal([]byte("")))
					})

				err = fs.Upload(ctx, uploadRef, ioutil.NopCloser(bytes.NewReader(fileContent)))

				Expect(err).ToNot(HaveOccurred())
				bs.AssertCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything)

				resources, err := fs.ListFolder(ctx, rootRef, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(2)) // .space folder & uploaded file
				Expect(resources[0].Path).To(Or(Equal(ref.Path), Equal("/.space")))
				Expect(resources[1].Path).To(Or(Equal(ref.Path), Equal("/.space")))
			})
		})

		When("the user tries to upload a file without intialising the upload", func() {
			It("fails", func() {
				var (
					fileContent = []byte("0123456789")
				)

				uploadRef := &provider.Reference{Path: "/some-non-existent-upload-reference"}
				err := fs.Upload(ctx, uploadRef, ioutil.NopCloser(bytes.NewReader(fileContent)))

				Expect(err).To(HaveOccurred())

				resources, err := fs.ListFolder(ctx, rootRef, []string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(resources)).To(Equal(1)) // .space folder
				Expect(resources[0].Path).To(Equal("/.space"))
			})
		})

	})
})
