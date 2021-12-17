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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Recycle", func() {
	var (
		env       *helpers.TestEnv
		projectID *provider.ResourceId
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with sufficient permissions", func() {
		BeforeEach(func() {
		})

		When("a user deletes files from the same space", func() {
			JustBeforeEach(func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(2)
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/subdir1",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("they are stored in the same trashbin", func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(1)
				items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(2))
			})

			It("they do not count towards the quota anymore", func() {
				env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).Return(provider.ResourcePermissions{GetQuota: true}, nil).Times(1)
				_, used, err := env.Fs.GetQuota(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes})
				Expect(err).ToNot(HaveOccurred())
				Expect(used).To(Equal(uint64(0)))
			})

			It("they can be permanently deleted by this user", func() {
				env.Blobstore.On("Delete", mock.Anything).Return(nil).Times(2)
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(4)

				items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(2))

				err = env.Fs.PurgeRecycleItem(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, items[0].Key, "/")
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.PurgeRecycleItem(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, items[1].Key, "/")
				Expect(err).ToNot(HaveOccurred())

				items, err = env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(0))
			})
		})

		When("two users delete files from the same space", func() {
			var ctx context.Context

			BeforeEach(func() {
				ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
					Id: &userpb.UserId{
						Idp:      "anotheridp",
						OpaqueId: "anotheruserid",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Username: "anotherusername",
				})
			})

			JustBeforeEach(func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(2)
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.Delete(ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/subdir1",
				})
				Expect(err).ToNot(HaveOccurred())

			})

			It("they are stored in the same trashbin (for both users)", func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(2)
				itemsA, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(itemsA)).To(Equal(2))

				itemsB, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(itemsB)).To(Equal(2))

				Expect(itemsA).To(Equal(itemsB))
			})

			It("they can be permanently deleted by the other user", func() {
				env.Blobstore.On("Delete", mock.Anything).Return(nil).Times(2)
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(4)

				items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(2))

				// pick correct ctx
				var ctx1, ctx2 context.Context
				switch items[0].Type {
				case provider.ResourceType_RESOURCE_TYPE_FILE:
					ctx1 = env.Ctx
					ctx2 = ctx
				case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
					ctx1 = ctx
					ctx2 = env.Ctx
				}

				err = env.Fs.PurgeRecycleItem(ctx1, &provider.Reference{ResourceId: env.SpaceRootRes}, items[0].Key, "/")
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.PurgeRecycleItem(ctx2, &provider.Reference{ResourceId: env.SpaceRootRes}, items[1].Key, "/")
				Expect(err).ToNot(HaveOccurred())

				items, err = env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(0))
			})
		})

		When("a user deletes files from different spaces", func() {
			BeforeEach(func() {
				var err error
				projectID, err = env.CreateTestStorageSpace("project")
				Expect(err).ToNot(HaveOccurred())
				Expect(projectID).ToNot(BeNil())
			})

			JustBeforeEach(func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(2)
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: projectID,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("they are stored in different trashbins", func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Times(2)
				items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(1))
				recycled1 := items[0]

				items, err = env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: projectID}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(1))
				recycled2 := items[0]

				Expect(recycled1).ToNot(Equal(recycled2))
			})
		})
	})
	Context("with insufficient permissions", func() {
		When("a user who can only read from a drive", func() {
			var ctx context.Context
			BeforeEach(func() {
				ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
					Id: &userpb.UserId{
						Idp:      "readidp",
						OpaqueId: "readuserid",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Username: "readusername",
				})

				// need user with access ...
				env.Permissions.On("HasPermission",
					mock.MatchedBy(func(ctx context.Context) bool {
						return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "userid"
					}),
					mock.Anything,
					mock.Anything,
				).Return(true, nil)

				// and user with read access ...
				env.Permissions.On("HasPermission",
					mock.MatchedBy(func(ctx context.Context) bool {
						return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "readuserid"
					}),
					mock.Anything,
					mock.MatchedBy(func(f func(*provider.ResourcePermissions) bool) bool {
						return f(&provider.ResourcePermissions{ListRecycle: true})
					}),
				).Return(true, nil)
				env.Permissions.On("HasPermission",
					mock.MatchedBy(func(ctx context.Context) bool {
						return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "readuserid"
					}),
					mock.Anything,
					mock.MatchedBy(func(f func(*provider.ResourcePermissions) bool) bool {
						return f(&provider.ResourcePermissions{
							PurgeRecycle: true,
							Delete:       true,
						})
					}),
				).Return(false, nil)
			})
			It("can list the trashbin", func() {
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				items, err := env.Fs.ListRecycle(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(1))
			})

			It("cannot delete files", func() {
				err := env.Fs.Delete(ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("permission denied"))

				items, err := env.Fs.ListRecycle(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(0))
			})

			It("cannot purge files from trashbin", func() {
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				items, err := env.Fs.ListRecycle(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(items)).To(Equal(1))

				err = env.Fs.PurgeRecycleItem(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, items[0].Key, "/")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})
		})
	})

	When("a user who cannot read from a drive", func() {
		var ctx context.Context
		BeforeEach(func() {
			ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
				Id: &userpb.UserId{
					Idp:      "maliciousidp",
					OpaqueId: "hacker",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Username: "mrhacker",
			})
			env.Permissions.On("HasPermission",
				mock.MatchedBy(func(ctx context.Context) bool {
					return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "userid"
				}),
				mock.Anything,
				mock.Anything,
			).Return(true, nil)
			env.Permissions.On("HasPermission",
				mock.MatchedBy(func(ctx context.Context) bool {
					return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "hacker"
				}),
				mock.Anything,
				mock.Anything,
			).Return(false, nil)
		})

		It("cannot delete, list or purge", func() {
			err := env.Fs.Delete(ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))

			err = env.Fs.Delete(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = env.Fs.ListRecycle(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))

			items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(items)).To(Equal(1))

			err = env.Fs.PurgeRecycleItem(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, items[0].Key, "/")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
		})
	})
})
