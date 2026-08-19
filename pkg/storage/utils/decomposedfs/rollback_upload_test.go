package decomposedfs_test

import (
	"os"

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
			Expect(env.Fs.RollbackUpload(env.Ctx, ref, "session-overwrite", storage.RollbackInfo{NodeExisted: true, SizeDiff: result.SizeDiff})).To(Succeed())

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
			Expect(env.Fs.RollbackUpload(env.Ctx, ref, "session-a", storage.RollbackInfo{NodeExisted: true, SizeDiff: 10})).To(Succeed())

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
			Expect(env.Fs.RollbackUpload(env.Ctx, missingRef, "any-session-id", storage.RollbackInfo{})).To(Succeed())
		})
	})

	// A node whose metadata is gone but whose file remains, e.g. because an
	// ancestor was trashed while the upload was in flight. ReadNode fails on the
	// missing parent id, so the rollback has only the session to go on.
	Context("orphaned node: the metadata is unreadable", func() {
		var (
			idRef    *provider.Reference
			nodePath string
			dir1Ref  *provider.Reference
			info     storage.RollbackInfo
		)

		// parentSize is the size dir1 accounts for, which the propagation moves.
		parentSize := func() uint64 {
			parentInfo, err := env.Fs.GetMD(env.Ctx, dir1Ref, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			return parentInfo.Size
		}

		// orphan prepares an upload, then purges the target's metadata. It returns
		// the parent size from before the optimistic propagation.
		orphan := func() uint64 {
			dir1Ref = &provider.Reference{ResourceId: env.SpaceRootRes, Path: "/dir1"}
			sizeBefore := parentSize()

			_, err := env.Fs.TouchFile(env.Ctx, ref, false, "")
			Expect(err).ToNot(HaveOccurred())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			nodePath = n.InternalPath()
			// Address the node by id: the path walk needs the parent metadata the
			// purge below destroys.
			idRef = &provider.Reference{ResourceId: &provider.ResourceId{
				StorageId: env.SpaceRootRes.GetStorageId(),
				SpaceId:   env.SpaceRootRes.GetSpaceId(),
				OpaqueId:  n.ID,
			}}

			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-orphan", storage.UploadInfo{
				NodeExisted: false,
				Size:        30,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(env.Fs.MarkProcessing(env.Ctx, ref, true, "session-orphan")).To(Succeed())
			Expect(parentSize()).To(Equal(sizeBefore+30), "the optimistic size should be propagated")

			// Purge, not remove: the cached attributes have to go too.
			Expect(env.Lookup.MetadataBackend().Purge(env.Ctx, nodePath)).To(Succeed())
			_, err = env.Lookup.NodeFromResource(env.Ctx, idRef)
			Expect(err).To(HaveOccurred(), "the node should be unreadable")

			info = storage.RollbackInfo{
				SizeDiff: result.SizeDiff,
				NodeID:   idRef.GetResourceId().GetOpaqueId(),
				ParentID: n.ParentID,
				Filename: "rollback-target.txt",
				Size:     30,
			}
			return sizeBefore
		}

		It("releases the quota and removes the unreachable node", func() {
			sizeBefore := orphan()

			Expect(env.Fs.RollbackUpload(env.Ctx, idRef, "session-orphan", info)).To(Succeed())

			Expect(parentSize()).To(Equal(sizeBefore))

			_, err := os.Stat(nodePath)
			Expect(err).To(HaveOccurred(), "the orphaned node should be gone")
		})

		// Without them there is no way to reach the parent, so report the failure
		// rather than pretending the upload was rolled back.
		It("reports the lookup failure when the session carries no ids", func() {
			orphan()

			err := env.Fs.RollbackUpload(env.Ctx, idRef, "session-orphan", storage.RollbackInfo{SizeDiff: 30})

			Expect(err).To(MatchError(ContainSubstring("node lookup failed")))
			_, statErr := os.Stat(nodePath)
			Expect(statErr).ToNot(HaveOccurred(), "the node should be left for a retry")
		})

		// The space is what the propagation walks up to, so there is nothing to
		// walk without it.
		It("reports the lookup failure when the reference names no space", func() {
			orphan()

			err := env.Fs.RollbackUpload(env.Ctx, &provider.Reference{Path: "/dir1/rollback-target.txt"}, "session-orphan", info)

			Expect(err).To(MatchError(ContainSubstring("node lookup failed")))
			_, statErr := os.Stat(nodePath)
			Expect(statErr).ToNot(HaveOccurred(), "the node should be left for a retry")
		})

		// Removing the node first would destroy the parent id the retry needs.
		It("keeps the node when the quota cannot be released", func() {
			orphan()
			// A parent id pointing nowhere fails the walk up the tree.
			info.ParentID = "does-not-exist"

			err := env.Fs.RollbackUpload(env.Ctx, idRef, "session-orphan", info)

			Expect(err).To(MatchError(ContainSubstring("could not revert propagate")))
			_, statErr := os.Stat(nodePath)
			Expect(statErr).ToNot(HaveOccurred(), "the node should be left for a retry")
		})
	})
})
