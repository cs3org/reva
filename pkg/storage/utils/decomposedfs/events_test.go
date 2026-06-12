package decomposedfs_test

import (
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	helpers "github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/owncloud/reva/v2/pkg/storagespace"
)

var _ = Describe("Storage event SpaceOwner propagation", func() {
	var (
		env *helpers.TestEnv
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("CreateDir", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					CreateContainer: true,
					Stat:            true,
				}, nil)

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.CreateDir(ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/events-test-createdir",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})

	Describe("TouchFile", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					InitiateFileUpload: true,
					Stat:               true,
				}, nil)

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.TouchFile(ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/events-touch-file.txt",
			}, false, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})

	Describe("SetLock", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					InitiateFileUpload: true,
					Stat:               true,
				}, nil)

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.SetLock(ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			}, &provider.Lock{
				Type:   provider.LockType_LOCK_TYPE_EXCL,
				LockId: "events-test-lock-id",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})

	Describe("Unlock", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					InitiateFileUpload: true,
					Stat:               true,
				}, nil)

			ref := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			}
			lock := &provider.Lock{
				Type:   provider.LockType_LOCK_TYPE_EXCL,
				LockId: "events-unlock-test-lock",
			}

			_, err := env.Fs.SetLock(env.Ctx, ref, lock)
			Expect(err).ToNot(HaveOccurred())

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.Unlock(ctx, ref, lock)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})

	Describe("RestoreRecycleItem", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			registerPermissions(env.Permissions, helpers.OwnerID, &provider.ResourcePermissions{
				Stat:               true,
				InitiateFileUpload: true,
				Delete:             true,
				ListRecycle:        true,
				RestoreRecycleItem: true,
			})

			err := env.Fs.Delete(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			items, err := env.Fs.ListRecycle(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, "", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(items)).To(BeNumerically(">=", 1))

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.RestoreRecycleItem(ctx, &provider.Reference{ResourceId: env.SpaceRootRes}, items[0].Key, "", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})

	Describe("RestoreRevision", func() {
		It("returns a WriteResult with SpaceOwner and propagates it via context slot", func() {
			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					Stat:               true,
					InitiateFileUpload: true,
					ListFileVersions:   true,
					RestoreFileVersion: true,
				}, nil)

			fileRef := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			}

			ri, err := env.Fs.GetMD(env.Ctx, fileRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ri).ToNot(BeNil())

			nodeID := ri.Id.OpaqueId
			spaceID := ri.Id.SpaceId
			nodePath := env.Lookup.InternalPath(spaceID, nodeID)

			revisionPath := nodePath + ".REV." + "2024-01-01T00:00:00.000000000Z"
			revFile, err := os.Create(revisionPath)
			Expect(err).ToNot(HaveOccurred())
			revFile.Close()

			err = env.Lookup.MetadataBackend().SetMultiple(env.Ctx, revisionPath,
				map[string][]byte{prefixes.BlobsizeAttr: []byte("0")},
				false)
			Expect(err).ToNot(HaveOccurred())

			revisions, err := env.Fs.ListRevisions(env.Ctx, fileRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(revisions)).To(BeNumerically(">=", 1))

			ctx := storagespace.ContextRegisterSpaceOwnerSlot(env.Ctx)

			res, err := env.Fs.RestoreRevision(ctx, fileRef, revisions[0].Key)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SpaceOwner).ToNot(BeNil())

			storagespace.ContextSetSpaceOwner(ctx, res.SpaceOwner)

			Expect(storagespace.ContextGetSpaceOwner(ctx)).To(Equal(res.SpaceOwner))
		})
	})
})
