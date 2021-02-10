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
	"context"
	"io/ioutil"
	"os"
	"strings"

	"github.com/stretchr/testify/mock"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decomposed", func() {
	var (
		ref  *provider.Reference
		user *userpb.User
		ctx  context.Context

		config      map[string]interface{}
		o           *options.Options
		lookup      *decomposedfs.Lookup
		permissions *mocks.PermissionsChecker
		bs          *treemocks.Blobstore
		fs          storage.FS
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: "foo",
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

		tmpRoot, err := ioutil.TempDir("", "reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		config = map[string]interface{}{
			"root":         tmpRoot,
			"enable_home":  true,
			"share_folder": "/Shares",
		}
		o, err = options.New(config)
		Expect(err).ToNot(HaveOccurred())
		lookup = &decomposedfs.Lookup{Options: o}
		permissions = &mocks.PermissionsChecker{}
		bs = &treemocks.Blobstore{}
	})

	JustBeforeEach(func() {
		var err error
		tree := tree.New(o.Root, true, true, lookup, bs)
		fs, err = decomposedfs.New(o, lookup, permissions, tree)
		Expect(err).ToNot(HaveOccurred())
		Expect(fs.CreateHome(ctx)).To(Succeed())
	})

	AfterEach(func() {
		root := o.Root
		if strings.HasPrefix(root, os.TempDir()) {
			os.RemoveAll(root)
		}
	})

	Describe("NewDefault", func() {
		It("works", func() {
			_, err := decomposedfs.NewDefault(map[string]interface{}{
				"root": o.Root,
			}, bs)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			_, err := helpers.CreateEmptyNode(ctx, "foo", "foo", user.Id, lookup)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("with insufficient permissions", func() {
			It("returns an error", func() {
				permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

				err := fs.Delete(ctx, ref)

				Expect(err).To(MatchError(ContainSubstring("permission denied")))
			})
		})

		Context("with sufficient permissions", func() {
			JustBeforeEach(func() {
				permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
			})

			It("does not (yet) delete the blob from the blobstore", func() {
				err := fs.Delete(ctx, ref)

				Expect(err).ToNot(HaveOccurred())
				bs.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
			})
		})
	})
})
