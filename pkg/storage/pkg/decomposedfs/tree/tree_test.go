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
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/google/uuid"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/lookup"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/prefixes"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	helpers "github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/testhelpers"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/tree"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tree", func() {
	var (
		env *helpers.TestEnv

		t *tree.Tree
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
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
			n, err = env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       originalPath,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("Delete", func() {
			Context("when the file was locked", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(n.InternalPath())
					Expect(err).ToNot(HaveOccurred())

					lock := &provider.Lock{
						Type:   provider.LockType_LOCK_TYPE_EXCL,
						User:   env.Owner.Id,
						LockId: uuid.New().String(),
					}
					Expect(n.SetLock(env.Ctx, lock)).To(Succeed())
					Expect(t.Delete(env.Ctx, n)).To(Succeed())

					_, err = os.Stat(n.InternalPath())
					Expect(err).To(HaveOccurred())
				})

				It("also removes the lock file", func() {
					_, err := os.Stat(n.LockFilePath())
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the file was not locked", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(n.InternalPath())
					Expect(err).ToNot(HaveOccurred())

					Expect(t.Delete(env.Ctx, n)).To(Succeed())

					_, err = os.Stat(n.InternalPath())
					Expect(err).To(HaveOccurred())
				})

				It("moves the file to the trash", func() {
					trashPath := path.Join(env.Root, "spaces", lookup.Pathify(n.SpaceRoot.ID, 1, 2), "trash", lookup.Pathify(n.ID, 4, 2))
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())
				})

				It("removes the file from its original location", func() {
					_, err := os.Stat(n.InternalPath())
					Expect(err).To(HaveOccurred())
				})

				It("sets the trash origin xattr", func() {
					trashPath := path.Join(env.Root, "spaces", lookup.Pathify(n.SpaceRoot.ID, 1, 2), "trash", lookup.Pathify(n.ID, 4, 2))
					resolveTrashPath, err := filepath.EvalSymlinks(trashPath)
					Expect(err).ToNot(HaveOccurred())

					parts := strings.SplitN(resolveTrashPath, "/nodes/", 2)
					trashID := strings.ReplaceAll(parts[1], "/", "")
					trashNode := node.NewBaseNode(n.SpaceID, trashID, env.Lookup)

					attr, err := env.Lookup.MetadataBackend().Get(env.Ctx, trashNode, prefixes.TrashOriginAttr)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(attr)).To(Equal("/dir1/file1"))
				})

				It("does not delete the blob from the blobstore", func() {
					env.Blobstore.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("*node.Node"))
				})
			})
		})

		Context("that was deleted", func() {
			var (
				trashPath string
			)

			JustBeforeEach(func() {
				env.Blobstore.On("Delete", mock.AnythingOfType("*node.Node")).Return(nil)
				trashPath = path.Join(env.Root, "spaces", lookup.Pathify(n.SpaceRoot.ID, 1, 2), "trash", lookup.Pathify(n.ID, 4, 2))
				Expect(t.Delete(env.Ctx, n)).To(Succeed())
			})

			Describe("PurgeRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())

					_, purgeFunc, err := t.PurgeRecycleItemFunc(env.Ctx, n.SpaceRoot.ID, n.ID, "")
					Expect(err).ToNot(HaveOccurred())
					Expect(purgeFunc()).To(Succeed())
				})

				It("removes the file from the trash", func() {
					_, err := os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})

				It("deletes the blob from the blobstore", func() {
					env.Blobstore.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("*node.Node"))
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
					_, _, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, n.SpaceRoot.ID, n.ID, "", nil)
					Expect(err).ToNot(HaveOccurred())

					Expect(restoreFunc()).To(Succeed())

					originalNode, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       originalPath,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(originalNode.Exists).To(BeTrue())
				})

				It("restores files to different locations", func() {
					ref := &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "dir1/newLocation",
					}
					dest, err := env.Lookup.NodeFromResource(env.Ctx, ref)
					Expect(err).ToNot(HaveOccurred())

					_, _, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, n.SpaceRoot.ID, n.ID, "", dest)
					Expect(err).ToNot(HaveOccurred())

					Expect(restoreFunc()).To(Succeed())

					newNode, err := env.Lookup.NodeFromResource(env.Ctx, ref)
					Expect(err).ToNot(HaveOccurred())
					Expect(newNode.Exists).To(BeTrue())

					ref.Path = originalPath
					originalNode, err := env.Lookup.NodeFromResource(env.Ctx, ref)
					Expect(err).ToNot(HaveOccurred())
					Expect(originalNode.Exists).To(BeFalse())
				})

				It("removes the file from the trash", func() {
					_, _, restoreFunc, err := t.RestoreRecycleItemFunc(env.Ctx, n.SpaceRoot.ID, n.ID, "", nil)
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
			n, err = env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "emptydir",
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("TouchFile", func() {
			It("creates a file inside", func() {
				ref := &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "emptydir/newFile",
				}
				fileToBeCreated, err := env.Lookup.NodeFromResource(env.Ctx, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(fileToBeCreated.Exists).To(BeFalse())

				err = t.TouchFile(env.Ctx, fileToBeCreated, false, "")
				Expect(err).ToNot(HaveOccurred())

				existingFile, err := env.Lookup.NodeFromResource(env.Ctx, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(existingFile.Exists).To(BeTrue())
			})
		})

		Context("that was deleted", func() {
			var (
				trashPath string
			)

			JustBeforeEach(func() {
				trashPath = path.Join(env.Root, "spaces", lookup.Pathify(n.SpaceRoot.ID, 1, 2), "trash", lookup.Pathify(n.ID, 4, 2))
				Expect(t.Delete(env.Ctx, n)).To(Succeed())

				env.Blobstore.On("Delete", mock.Anything).Return(nil)
			})

			Describe("PurgeRecycleItemFunc", func() {
				JustBeforeEach(func() {
					_, err := os.Stat(trashPath)
					Expect(err).ToNot(HaveOccurred())

					_, purgeFunc, err := t.PurgeRecycleItemFunc(env.Ctx, n.SpaceRoot.ID, n.ID, "")
					Expect(err).ToNot(HaveOccurred())
					Expect(purgeFunc()).To(Succeed())
				})

				It("removes the file from the trash", func() {
					_, err := os.Stat(trashPath)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Describe("Propagate", func() {
		var dir *node.Node

		JustBeforeEach(func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
				CreateContainer: true,
				Stat:            true,
			}, nil)

			// Create test dir
			var err error
			dir, err = env.CreateTestDir("testdir", &provider.Reference{ResourceId: env.SpaceRootRes})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("with TreeTimeAccounting enabled", func() {
			It("sets the tmtime of the parent", func() {
				file, err := env.CreateTestFile("file1", "", dir.ID, dir.SpaceID, 1)
				Expect(err).ToNot(HaveOccurred())

				perms := node.OwnerPermissions()
				riBefore, err := dir.AsResourceInfo(env.Ctx, perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())

				err = env.Tree.Propagate(env.Ctx, file, 0)
				Expect(err).ToNot(HaveOccurred())

				dir, err := env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				riAfter, err := dir.AsResourceInfo(env.Ctx, perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(riAfter.Etag).ToNot(Equal(riBefore.Etag))
			})
		})

		Describe("with TreeSizeAccounting enabled", func() {
			It("calculates the size", func() {
				_, err := env.CreateTestFile("file1", "", dir.ID, dir.SpaceID, 1)
				Expect(err).ToNot(HaveOccurred())

				dir, err := env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize(env.Ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(1)))
			})

			It("considers all files", func() {
				_, err := env.CreateTestFile("file1", "", dir.ID, dir.SpaceID, 1)
				Expect(err).ToNot(HaveOccurred())
				_, err = env.CreateTestFile("file2", "", dir.ID, dir.SpaceID, 100)
				Expect(err).ToNot(HaveOccurred())

				dir, err := env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize(env.Ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(101)))
			})

			It("adds the size of child directories", func() {
				subdir, err := env.CreateTestDir("testdir/200bytes", &provider.Reference{ResourceId: env.SpaceRootRes})
				Expect(err).ToNot(HaveOccurred())
				err = subdir.SetTreeSize(env.Ctx, uint64(200))
				Expect(err).ToNot(HaveOccurred())
				err = env.Tree.Propagate(env.Ctx, subdir, 200)
				Expect(err).ToNot(HaveOccurred())

				_, err = env.CreateTestFile("file1", "", dir.ID, dir.SpaceID, 1)
				Expect(err).ToNot(HaveOccurred())

				dir, err := env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize(env.Ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(201)))
			})

			It("stops at nodes with no propagation flag", func() {
				subdir, err := env.CreateTestDir("testdir/200bytes", &provider.Reference{ResourceId: env.SpaceRootRes})
				Expect(err).ToNot(HaveOccurred())
				err = subdir.SetTreeSize(env.Ctx, uint64(200))
				Expect(err).ToNot(HaveOccurred())
				err = env.Tree.Propagate(env.Ctx, subdir, 200)
				Expect(err).ToNot(HaveOccurred())

				dir, err := env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				size, err := dir.GetTreeSize(env.Ctx)
				Expect(size).To(Equal(uint64(200)))
				Expect(err).ToNot(HaveOccurred())

				stopdir, err := env.CreateTestDir("testdir/stophere", &provider.Reference{ResourceId: env.SpaceRootRes})
				Expect(err).ToNot(HaveOccurred())
				err = stopdir.SetXattrString(env.Ctx, prefixes.PropagationAttr, "0")
				Expect(err).ToNot(HaveOccurred())
				otherdir, err := env.CreateTestDir("testdir/stophere/lotsofbytes", &provider.Reference{ResourceId: env.SpaceRootRes})
				Expect(err).ToNot(HaveOccurred())
				err = otherdir.SetTreeSize(env.Ctx, uint64(100000))
				Expect(err).ToNot(HaveOccurred())
				err = env.Tree.Propagate(env.Ctx, otherdir, 100000)
				Expect(err).ToNot(HaveOccurred())

				dir, err = env.Lookup.NodeFromID(env.Ctx, &provider.ResourceId{
					StorageId: dir.SpaceID,
					SpaceId:   dir.SpaceID,
					OpaqueId:  dir.ID,
				})
				Expect(err).ToNot(HaveOccurred())
				size, err = dir.GetTreeSize(env.Ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(size).To(Equal(uint64(200)))
			})
		})
	})

	DescribeTable("ReadSpaceAndNodeFromIndexLink",
		func(link string, expectSpace string, expectedNode string, shouldErr bool) {
			space, node, err := tree.ReadSpaceAndNodeFromIndexLink(link)
			if shouldErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(space).To(Equal(expectSpace))
			Expect(node).To(Equal(expectedNode))
		},

		Entry("invalid number of slashes", "../../../spaces/sp_ace-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("does not contain spaces", "../../../spac_s/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("does not contain nodes", "../../../spaces/sp/ace-id/nod_s/sh/or/tn/od/eid", "", "", true),
		Entry("does not start with ..", "_./../../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("does not start with ../..", "../_./../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("does not start with ../../..", "../_./../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("invalid", "../../../spaces/space-id/nodes/sh/or/tn/od/eid", "", "", true),
		Entry("uuid", "../../../spaces/4c/510ada-c86b-4815-8820-42cdf82c3d51/nodes/4c/51/0a/da/-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51", false),
		Entry("uuid", "../../../spaces/4c/510ada-c86b-4815-8820-42cdf82c3d51/nodes/4c/51/0a/da/-c86b-4815-8820-42cdf82c3d51.T.2022-02-24T12:35:18.196484592Z", "4c510ada-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51.T.2022-02-24T12:35:18.196484592Z", false),
		Entry("short", "../../../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "space-id", "shortnodeid", false),
	)
})
