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

	Context("with sufficent permissions", func() {
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
	Context("with insufficent permissions", func() {
		When("a user can only read from a drive", func() {
			//var ctx context.Context
			BeforeEach(func() {
				//ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
				//Id: &userpb.UserId{
				//Idp:      "readidp",
				//OpaqueId: "readuserid",
				//Type:     userpb.UserType_USER_TYPE_PRIMARY,
				//},
				//Username: "readusername",
				//})

				env.Permissions.On("HasPermission", mock.MatchedBy(func(ctx context.Context) bool {
					return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId == "userid"
				}), mock.Anything, mock.Anything).Return(true, nil)

				//c := env.Permissions.On("HasPermission", mock.MatchedBy(func(ctx context.Context) bool {
				//return ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId != "userid"
				//}), mock.Anything, mock.Anything)
				//c.Return(c.Arguments.Get(1).(func(*provider.ResourcePermissions) bool)(&provider.ResourcePermissions{}))
			})
			It("he can list the trashbin", func() {
				err := env.Fs.Delete(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())

				//items, err := env.Fs.ListRecycle(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "/")
				//Expect(err).ToNot(HaveOccurred())
				//Expect(len(items)).To(Equal(1))
			})
		})
	})

})
