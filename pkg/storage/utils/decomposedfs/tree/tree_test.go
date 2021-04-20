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
			n            *node.Node
			originalPath = "dir1/file1"
		)

		JustBeforeEach(func() {
			var err error
			n, err = env.Lookup.NodeFromPath(env.Ctx, originalPath)
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
				Expect(string(attr)).To(Equal("/dir1/file1"))
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
				env.Blobstore.On("Delete", n.BlobID).Return(nil)
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
				})

				It("restores the file to its original location if the targetPath is empty", func() {
					_, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, env.Owner.Id.OpaqueId+":"+n.ID, "")
					Expect(err).ToNot(HaveOccurred())

					Expect(restoreFunc()).To(Succeed())

					originalNode, err := env.Lookup.NodeFromPath(env.Ctx, originalPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(originalNode.Exists).To(BeTrue())
				})

				It("restores files to different locations", func() {
					_, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, env.Owner.Id.OpaqueId+":"+n.ID, "dir1/newLocation")
					Expect(err).ToNot(HaveOccurred())

					Expect(restoreFunc()).To(Succeed())

					newNode, err := env.Lookup.NodeFromPath(env.Ctx, "dir1/newLocation")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNode.Exists).To(BeTrue())

					originalNode, err := env.Lookup.NodeFromPath(env.Ctx, originalPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(originalNode.Exists).To(BeFalse())
				})

				It("removes the file from the trash", func() {
					_, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, env.Owner.Id.OpaqueId+":"+n.ID, "")
					Expect(err).ToNot(HaveOccurred())

					Expect(restoreFunc()).To(Succeed())

					_, err = os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Context("with an empty directory", func() {
		var (
			n *node.Node
		)

		JustBeforeEach(func() {
			var err error
			n, err = env.Lookup.NodeFromPath(env.Ctx, "emptydir")
			Expect(err).ToNot(HaveOccurred())
		})

		Context("that was deleted", func() {
			var (
				trashPath string
			)

			JustBeforeEach(func() {
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

				It("does not try to delete a blob from the blobstore", func() {
					env.Blobstore.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
				})
			})
		})
	})

	Describe("Propagate", func() {
		var dir *node.Node

		JustBeforeEach(func() {
			env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)

			// Create test dir
			var err error
			dir, err = env.CreateTestDir("testdir")
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("with TreeTimeAccounting enabled", func() {
			It("sets the tmtime of the parent", func() {
				file, err := env.CreateTestFile("file1", "", 1, dir.ID)
				Expect(err).ToNot(HaveOccurred())

				riBefore, err := dir.AsResourceInfo(env.Ctx, node.OwnerPermissions, []string{})
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, file)
				Expect(err).ToNot(HaveOccurred())

				riAfter, err := dir.AsResourceInfo(env.Ctx, node.OwnerPermissions, []string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(riAfter.Etag).ToNot(Equal(riBefore.Etag))
			})
		})

		Describe("with TreeSizeAccounting enabled", func() {
			It("calculates the size", func() {
				file, err := env.CreateTestFile("file1", "", 1, dir.ID)
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, file)
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize()
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(1)))
			})

			It("considers all files", func() {
				_, err := env.CreateTestFile("file1", "", 1, dir.ID)
				Expect(err).ToNot(HaveOccurred())
				file2, err := env.CreateTestFile("file2", "", 100, dir.ID)
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, file2)
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize()
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(101)))
			})

			It("adds the size of child directories", func() {
				subdir, err := env.CreateTestDir("testdir/200bytes")
				Expect(err).ToNot(HaveOccurred())
				err = subdir.SetTreeSize(uint64(200))
				Expect(err).ToNot(HaveOccurred())

				file, err := env.CreateTestFile("file1", "", 1, dir.ID)
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, file)
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize()
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(201)))
			})

			It("stops at nodes with no propagation flag", func() {
				subdir, err := env.CreateTestDir("testdir/200bytes")
				Expect(err).ToNot(HaveOccurred())
				err = subdir.SetTreeSize(uint64(200))
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, subdir)
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize()
				Expect(size).To(Equal(uint64(200)))
				Expect(err).ToNot(HaveOccurred())

				stopdir, err := env.CreateTestDir("testdir/stophere")
				Expect(err).ToNot(HaveOccurred())
				err = xattr.Set(stopdir.InternalPath(), xattrs.PropagationAttr, []byte("0"))
				Expect(err).ToNot(HaveOccurred())
				otherdir, err := env.CreateTestDir("testdir/stophere/lotsofbytes")
				Expect(err).ToNot(HaveOccurred())
				err = otherdir.SetTreeSize(uint64(100000))
				Expect(err).ToNot(HaveOccurred())
				err = env.Tree.Propagate(env.Ctx, otherdir)
				Expect(err).ToNot(HaveOccurred())

				size, err = dir.GetTreeSize()
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(200)))
			})
		})
	})
})
