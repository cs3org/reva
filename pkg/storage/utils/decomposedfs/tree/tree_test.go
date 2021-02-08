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
	"os"
	"path"

	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tree", func() {
	var (
		env *helpers.TestEnv

		t *tree.Tree
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())
		t = env.Tree
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Context("with an existingfile", func() {
		var (
			n *node.Node
		)

		JustBeforeEach(func() {
			var err error
			n, err = env.Lookup.NodeFromPath(env.Ctx, "dir1")
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("Delete", func() {
			JustBeforeEach(func() {
				_, err := os.Stat(n.InternalPath())
				Expect(err).ToNot(HaveOccurred())

				Expect(t.Delete(env.Ctx, n)).To(Succeed())

				_, err = os.Stat(n.InternalPath())
				Expect(err).To(HaveOccurred())
			})

			It("moves the file to the trash", func() {
				trashPath := path.Join(env.Root, "trash", env.Owner.Id.OpaqueId, n.ID)
				_, err := os.Stat(trashPath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("removes the file from its original location", func() {
				_, err := os.Stat(n.InternalPath())
				Expect(err).To(HaveOccurred())
			})

			It("sets the trash origin xattr", func() {
				trashPath := path.Join(env.Root, "trash", env.Owner.Id.OpaqueId, n.ID)
				attr, err := xattr.Get(trashPath, xattrs.TrashOriginAttr)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(attr)).To(Equal(n.Name))
			})

			It("does not delete the blob from the blobstore", func() {
				env.Blobstore.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
			})
		})

		Context("that was deleted", func() {
			var (
				trashPath string
			)

			JustBeforeEach(func() {
				env.Blobstore.On("Delete", n.ID).Return(nil)
				trashPath = path.Join(env.Root, "trash", env.Owner.Id.OpaqueId, n.ID)
				Expect(t.Delete(env.Ctx, n)).To(Succeed())
			})

			Describe("PurgeRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())

					_, purgeFunc, err := t.PurgeRecycleItemFunc(env.Ctx, env.Owner.Id.OpaqueId+":"+n.ID)
					Expect(err).ToNot(HaveOccurred())
					Expect(purgeFunc()).To(Succeed())
				})

				It("removes the file from the trash", func() {
					_, err := os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})

				It("deletes the blob from the blobstore", func() {
					env.Blobstore.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
				})
			})

			Describe("RestoreRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())
					_, err = os.Stat(n.InternalPath())
					Expect(err).To(HaveOccurred())

					_, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, env.Owner.Id.OpaqueId+":"+n.ID)
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
