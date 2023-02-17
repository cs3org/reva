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

package node_test

import (
	"encoding/json"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ocsconv "github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/conversions"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/cs3org/reva/v2/pkg/storage/utils/grants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Node", func() {
	var (
		env *helpers.TestEnv

		id   string
		name string
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		id = "fooId"
		name = "foo"
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("New", func() {
		It("generates unique blob ids if none are given", func() {
			n1 := node.New(env.SpaceRootRes.SpaceId, id, "", name, 10, "", provider.ResourceType_RESOURCE_TYPE_FILE, env.Owner.Id, env.Lookup)
			n2 := node.New(env.SpaceRootRes.SpaceId, id, "", name, 10, "", provider.ResourceType_RESOURCE_TYPE_FILE, env.Owner.Id, env.Lookup)

			Expect(len(n1.BlobID)).To(Equal(36))
			Expect(n1.BlobID).ToNot(Equal(n2.BlobID))
		})
	})

	Describe("ReadNode", func() {
		It("reads the blobID from the xattrs", func() {
			lookupNode, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "./dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			n, err := node.ReadNode(env.Ctx, env.Lookup, lookupNode.SpaceID, lookupNode.ID, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(n.BlobID).To(Equal("file1-blobid"))
		})
	})

	Describe("WriteMetadata", func() {
		It("writes all xattrs", func() {
			ref := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			}
			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())

			blobsize := int64(239485734)
			n.Name = "TestName"
			n.BlobID = "TestBlobID"
			n.Blobsize = blobsize

			err = n.WriteAllNodeMetadata()
			Expect(err).ToNot(HaveOccurred())
			n2, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(n2.Name).To(Equal("TestName"))
			Expect(n2.BlobID).To(Equal("TestBlobID"))
			Expect(n2.Blobsize).To(Equal(blobsize))
		})
	})

	Describe("Parent", func() {
		It("returns the parent node", func() {
			child, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/subdir1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(child).ToNot(BeNil())

			parent, err := child.Parent()
			Expect(err).ToNot(HaveOccurred())
			Expect(parent).ToNot(BeNil())
			Expect(parent.ID).To(Equal(child.ParentID))
		})
	})
	Describe("Child", func() {
		var (
			parent *node.Node
		)

		JustBeforeEach(func() {
			var err error
			parent, err = env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(parent).ToNot(BeNil())
		})

		It("returns an empty node if the child does not exist", func() {
			child, err := parent.Child(env.Ctx, "does-not-exist")
			Expect(err).ToNot(HaveOccurred())
			Expect(child).ToNot(BeNil())
			Expect(child.Exists).To(BeFalse())
		})

		It("returns a directory node with all metadata", func() {
			child, err := parent.Child(env.Ctx, "subdir1")
			Expect(err).ToNot(HaveOccurred())
			Expect(child).ToNot(BeNil())
			Expect(child.Exists).To(BeTrue())
			Expect(child.ParentID).To(Equal(parent.ID))
			Expect(child.Name).To(Equal("subdir1"))
			Expect(child.Blobsize).To(Equal(int64(0)))
		})

		It("returns a file node with all metadata", func() {
			child, err := parent.Child(env.Ctx, "file1")
			Expect(err).ToNot(HaveOccurred())
			Expect(child).ToNot(BeNil())
			Expect(child.Exists).To(BeTrue())
			Expect(child.ParentID).To(Equal(parent.ID))
			Expect(child.Name).To(Equal("file1"))
			Expect(child.Blobsize).To(Equal(int64(1234)))
		})

		It("handles (broken) links including file segments by returning an non-existent node", func() {
			child, err := parent.Child(env.Ctx, "file1/broken")
			Expect(err).ToNot(HaveOccurred())
			Expect(child).ToNot(BeNil())
			Expect(child.Exists).To(BeFalse())
		})
	})

	Describe("AsResourceInfo", func() {
		var (
			n *node.Node
		)

		BeforeEach(func() {
			var err error
			n, err = env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("the Etag field", func() {
			It("is set", func() {
				perms := node.OwnerPermissions()
				ri, err := n.AsResourceInfo(env.Ctx, &perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ri.Etag)).To(Equal(34))
			})

			It("changes when the tmtime is set", func() {
				perms := node.OwnerPermissions()
				ri, err := n.AsResourceInfo(env.Ctx, &perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ri.Etag)).To(Equal(34))
				before := ri.Etag

				tmtime := time.Now()
				Expect(n.SetTMTime(&tmtime)).To(Succeed())

				ri, err = n.AsResourceInfo(env.Ctx, &perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ri.Etag)).To(Equal(34))
				Expect(ri.Etag).ToNot(Equal(before))
			})

			It("includes the lock in the Opaque", func() {
				lock := &provider.Lock{
					Type:   provider.LockType_LOCK_TYPE_EXCL,
					User:   env.Owner.Id,
					LockId: "foo",
				}
				err := n.SetLock(env.Ctx, lock)
				Expect(err).ToNot(HaveOccurred())

				perms := node.OwnerPermissions()
				ri, err := n.AsResourceInfo(env.Ctx, &perms, []string{}, []string{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(ri.Opaque).ToNot(BeNil())
				Expect(ri.Opaque.Map["lock"]).ToNot(BeNil())

				storedLock := &provider.Lock{}
				err = json.Unmarshal(ri.Opaque.Map["lock"].Value, storedLock)
				Expect(err).ToNot(HaveOccurred())
				Expect(storedLock).To(Equal(lock))
			})
		})
	})
	Describe("Permissions", func() {
		It("Checks the owner permissions on a personal space", func() {
			node1, err := env.Lookup.NodeFromSpaceID(env.Ctx, env.SpaceRootRes)
			Expect(err).ToNot(HaveOccurred())
			perms, _ := node1.PermissionSet(env.Ctx)
			Expect(perms).To(Equal(node.OwnerPermissions()))
		})
		It("Checks the manager permissions on a project space", func() {
			pSpace, err := env.CreateTestStorageSpace("project", &provider.Quota{QuotaMaxBytes: 2000})
			Expect(err).ToNot(HaveOccurred())
			nodePSpace, err := env.Lookup.NodeFromSpaceID(env.Ctx, pSpace)
			Expect(err).ToNot(HaveOccurred())
			u := ctxpkg.ContextMustGetUser(env.Ctx)
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
				UpdateGrant: true,
				Stat:        true,
			}, nil).Times(1)
			err = env.Fs.AddGrant(env.Ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{
					SpaceId:  pSpace.SpaceId,
					OpaqueId: pSpace.OpaqueId,
				},
			}, &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: u.Id,
					},
				},
				Permissions: ocsconv.NewManagerRole().CS3ResourcePermissions(),
			})
			Expect(err).ToNot(HaveOccurred())
			perms, _ := nodePSpace.PermissionSet(env.Ctx)
			expected := ocsconv.NewManagerRole().CS3ResourcePermissions()
			Expect(grants.PermissionsEqual(&perms, expected)).To(BeTrue())
		})
		It("Checks the Editor permissions on a project space and a denial", func() {
			storageSpace, err := env.CreateTestStorageSpace("project", &provider.Quota{QuotaMaxBytes: 2000})
			Expect(err).ToNot(HaveOccurred())
			spaceRoot, err := env.Lookup.NodeFromSpaceID(env.Ctx, storageSpace)
			Expect(err).ToNot(HaveOccurred())
			u := ctxpkg.ContextMustGetUser(env.Ctx)
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
				UpdateGrant: true,
				Stat:        true,
			}, nil).Times(1)
			err = env.Fs.AddGrant(env.Ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{
					SpaceId:  storageSpace.SpaceId,
					OpaqueId: storageSpace.OpaqueId,
				},
			}, &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: u.Id,
					},
				},
				Permissions: ocsconv.NewEditorRole().CS3ResourcePermissions(),
			})
			Expect(err).ToNot(HaveOccurred())
			permissionsActual, _ := spaceRoot.PermissionSet(env.Ctx)
			permissionsExpected := ocsconv.NewEditorRole().CS3ResourcePermissions()
			Expect(grants.PermissionsEqual(&permissionsActual, permissionsExpected)).To(BeTrue())
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
				Stat:            true,
				CreateContainer: true,
			}, nil).Times(1)
			subfolder, err := env.CreateTestDir("subpath", &provider.Reference{
				ResourceId: &provider.ResourceId{
					SpaceId:  storageSpace.SpaceId,
					OpaqueId: storageSpace.OpaqueId,
				},
				Path: ""},
			)
			Expect(err).ToNot(HaveOccurred())
			// adding a denial on the subpath
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
				DenyGrant: true,
				Stat:      true,
			}, nil).Times(1)
			err = env.Fs.AddGrant(env.Ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{
					SpaceId:  storageSpace.SpaceId,
					OpaqueId: storageSpace.OpaqueId,
				},
				Path: "subpath",
			}, &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: u.Id,
					},
				},
				Permissions: ocsconv.NewDeniedRole().CS3ResourcePermissions(),
			})
			Expect(err).ToNot(HaveOccurred())
			// checking that the path "subpath" is denied properly
			subfolder, err = node.ReadNode(env.Ctx, env.Lookup, subfolder.SpaceID, subfolder.ID, false)
			Expect(err).ToNot(HaveOccurred())
			subfolderActual, denied := subfolder.PermissionSet(env.Ctx)
			subfolderExpected := ocsconv.NewDeniedRole().CS3ResourcePermissions()
			Expect(grants.PermissionsEqual(&subfolderActual, subfolderExpected)).To(BeTrue())
			Expect(denied).To(BeTrue())
		})
	})

	Describe("SpaceOwnerOrManager", func() {
		It("returns the space owner", func() {
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			o := n.SpaceOwnerOrManager(env.Ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(o).To(Equal(env.Owner.Id))
		})

	})
})
