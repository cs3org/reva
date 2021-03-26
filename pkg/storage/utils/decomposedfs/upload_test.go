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
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/mock"

	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	ruser "github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("File uploads", func() {
	var (
		ref  *provider.Reference
		fs   storage.FS
		user *userpb.User
		ctx  context.Context

		o           *options.Options
		lookup      *decomposedfs.Lookup
		permissions *mocks.PermissionsChecker
		bs          *treemocks.Blobstore
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: "/foo",
			},
		}
		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid",
			},
			Username: "username",
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

	JustBeforeEach(func() {
		var err error
		tree := tree.New(o.Root, true, true, lookup, bs)
		fs, err = decomposedfs.New(o, lookup, permissions, tree)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with insufficient permissions", func() {
		BeforeEach(func() {
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		})

		Describe("InitiateUpload", func() {
			It("fails", func() {
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError("error: permission denied: root/foo"))
			})
		})
	})

	Context("with sufficient permissions", func() {
		BeforeEach(func() {
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
		})

		Describe("InitiateUpload", func() {
			It("returns uploadIds for simple and tus uploads", func() {
				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())
			})
		})

		Describe("Upload", func() {
			var (
				fileContent = []byte("0123456789")
			)

			It("stores the blob in the blobstore", func() {
				bs.On("Upload", mock.AnythingOfType("string"), mock.AnythingOfType("*os.File")).
					Return(nil).
					Run(func(args mock.Arguments) {
						reader := args.Get(1).(io.Reader)
						data, err := ioutil.ReadAll(reader)

						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal([]byte("0123456789")))
					})

				err := fs.Upload(ctx, ref, ioutil.NopCloser(bytes.NewReader(fileContent)))
				Expect(err).ToNot(HaveOccurred())

				bs.AssertCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything)
			})
		})
	})
})
