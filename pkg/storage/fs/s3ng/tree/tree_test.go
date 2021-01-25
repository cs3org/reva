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

package tree_test

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/tree"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/tree/mocks"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/xattrs"
	ruser "github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tree", func() {
	var (
		user *userpb.User
		ctx  context.Context

		blobstore *mocks.Blobstore
		lookup    tree.PathLookup
		options   *s3ng.Options

		t                  *tree.Tree
		treeTimeAccounting bool
		treeSizeAccounting bool
	)

	BeforeEach(func() {
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
		options = &s3ng.Options{}
		err = mapstructure.Decode(map[string]interface{}{
			"root": tmpRoot,
		}, options)
		Expect(err).ToNot(HaveOccurred())

		blobstore = &mocks.Blobstore{}
		lookup = &s3ng.Lookup{Options: options}
	})

	JustBeforeEach(func() {
		t = tree.New(options.Root, treeTimeAccounting, treeSizeAccounting, lookup, blobstore)
		Expect(t.Setup("root")).To(Succeed())
	})

	AfterEach(func() {
		root := options.Root
		if strings.HasPrefix(root, os.TempDir()) {
			os.RemoveAll(root)
		}
	})

	Describe("New", func() {
		It("returns a Tree instance", func() {
			Expect(t).ToNot(BeNil())
		})
	})

	Context("with an existingfile", func() {
		var (
			n *node.Node
		)

		JustBeforeEach(func() {
			n = createEmptyNode("fooId", "root", "fooName", user.Id, lookup)
			n.WriteMetadata(user.Id)
		})

		Describe("Delete", func() {
			JustBeforeEach(func() {
				_, err := os.Stat(n.InternalPath())
				Expect(err).ToNot(HaveOccurred())

				Expect(t.Delete(ctx, n)).To(Succeed())

				_, err = os.Stat(n.InternalPath())
				Expect(err).To(HaveOccurred())
			})

			It("moves the file to the trash", func() {
				trashPath := path.Join(options.Root, "trash", user.Id.OpaqueId, n.ID)
				_, err := os.Stat(trashPath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("removes the file from its original location", func() {
				_, err := os.Stat(n.InternalPath())
				Expect(err).To(HaveOccurred())
			})

			It("sets the trash origin xattr", func() {
				trashPath := path.Join(options.Root, "trash", user.Id.OpaqueId, n.ID)
				attr, err := xattr.Get(trashPath, xattrs.TrashOriginAttr)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(attr)).To(Equal(n.Name))
			})

			It("does not delete the blob from the blobstore", func() {
				blobstore.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
			})
		})

		Context("that was deleted", func() {
			var (
				trashPath string
			)

			BeforeEach(func() {
				blobstore.On("Delete", n.ID).Return(nil)
				trashPath = path.Join(options.Root, "trash", user.Id.OpaqueId, n.ID)
			})

			JustBeforeEach(func() {
				Expect(t.Delete(ctx, n)).To(Succeed())
			})

			Describe("PurgeRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())

					_, purgeFunc, err := t.PurgeRecycleItemFunc(ctx, user.Id.OpaqueId+":"+n.ID)
					Expect(err).ToNot(HaveOccurred())
					Expect(purgeFunc()).To(Succeed())
				})

				It("removes the file from the trash", func() {
					_, err := os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})

				It("deletes the blob from the blobstore", func() {
					blobstore.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
				})
			})

			Describe("RestoreRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())
					_, err = os.Stat(n.InternalPath())
					Expect(err).To(HaveOccurred())

					_, restoreFunc, err := t.RestoreRecycleItemFunc(ctx, user.Id.OpaqueId+":"+n.ID)
					Expect(err).ToNot(HaveOccurred())
					Expect(restoreFunc()).To(Succeed())
				})

				It("restores the file to its original location", func() {
					_, err := os.Stat(n.InternalPath())
					Expect(err).ToNot(HaveOccurred())
				})
				It("removes the file from the trash", func() {
					_, err := os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
})

func createEmptyNode(id, parent, name string, userid *userpb.UserId, lookup tree.PathLookup) *node.Node {
	n := node.New(id, parent, name, 0, userid, lookup)
	p, err := n.Parent()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	// Create an empty file node
	_, err = os.OpenFile(n.InternalPath(), os.O_CREATE, 0644)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	// ... and an according link in the parent
	err = os.Symlink("../"+n.ID, path.Join(p.InternalPath(), n.Name))
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	return n
}
