package decomposedfs_test

import (
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	helpers "github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("RollbackUpload", func() {
	var (
		env *helpers.TestEnv
		ref *provider.Reference
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).
			Return(&provider.ResourcePermissions{
				InitiateFileUpload: true,
				Stat:               true,
				ListFileVersions:   true,
			}, nil)

		ref = &provider.Reference{
			ResourceId: env.SpaceRootRes,
			Path:       "/dir1/rollback-target.txt",
		}
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Context("versioning enabled, existing node, session owns node", func() {
		It("restores previous version, removes version file, reverts size propagation", func() {
			_, err := env.Fs.TouchFile(env.Ctx, ref, false, "")
			Expect(err).ToNot(HaveOccurred())

			_, err = env.Fs.PrepareUpload(env.Ctx, ref, "session-init", storage.UploadInfo{
				NodeExisted: false,
				Size:        10,
			})
			Expect(err).ToNot(HaveOccurred())

			parentInfoBefore, err := env.Fs.GetMD(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes, Path: "/dir1"}, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			sizeBefore := parentInfoBefore.Size

			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-overwrite", storage.UploadInfo{
				NodeExisted: true,
				Size:        30,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.VersionCreated).To(BeTrue())
			Expect(result.SizeDiff).To(Equal(int64(20))) // 30 - 10

			revsBefore, err := env.Fs.ListRevisions(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(revsBefore).To(HaveLen(1))

			Expect(env.Fs.MarkProcessing(env.Ctx, ref, true, "session-overwrite")).To(Succeed())
			Expect(env.Fs.RollbackUpload(env.Ctx, ref, "session-overwrite", true, result.SizeDiff)).To(Succeed())

			revsAfter, err := env.Fs.ListRevisions(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(revsAfter).To(BeEmpty())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			blobID, err := n.Xattr(env.Ctx, prefixes.BlobIDAttr)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(blobID)).To(Equal("session-init"))

			parentInfoAfter, err := env.Fs.GetMD(env.Ctx, &provider.Reference{ResourceId: env.SpaceRootRes, Path: "/dir1"}, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(parentInfoAfter.Size).To(Equal(sizeBefore))
		})
	})

	Context("session guard: another session owns the node", func() {
		It("is a no-op and does not touch the node", func() {
			_, err := env.Fs.TouchFile(env.Ctx, ref, false, "")
			Expect(err).ToNot(HaveOccurred())

			_, err = env.Fs.PrepareUpload(env.Ctx, ref, "session-a", storage.UploadInfo{
				NodeExisted: false,
				Size:        10,
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = env.Fs.PrepareUpload(env.Ctx, ref, "session-b", storage.UploadInfo{
				NodeExisted: true,
				Size:        20,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(env.Fs.MarkProcessing(env.Ctx, ref, true, "session-b")).To(Succeed())
			Expect(env.Fs.RollbackUpload(env.Ctx, ref, "session-a", true, 10)).To(Succeed())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			blobID, err := n.Xattr(env.Ctx, prefixes.BlobIDAttr)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(blobID)).To(Equal("session-b"))
		})
	})

	Context("new node (NodeExists=false)", func() {
		It("returns nil without error", func() {
			missingRef := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/does-not-exist.txt",
			}
			Expect(env.Fs.RollbackUpload(env.Ctx, missingRef, "any-session-id", false, 0)).To(Succeed())
		})
	})
})
