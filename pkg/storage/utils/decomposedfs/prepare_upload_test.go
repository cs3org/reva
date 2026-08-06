package decomposedfs_test

import (
	"context"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/aspects"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("PrepareUpload", func() {
	var (
		env *helpers.TestEnv
		ref *provider.Reference
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		ref = &provider.Reference{
			ResourceId: env.SpaceRootRes,
			Path:       "/dir1/upload-target.txt",
		}
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	touchTarget := func() {
		env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).
			Return(&provider.ResourcePermissions{
				InitiateFileUpload: true,
				Stat:               true,
				ListFileVersions:   true,
			}, nil)
		_, err := env.Fs.TouchFile(env.Ctx, ref, false, "")
		Expect(err).ToNot(HaveOccurred())
	}

	Context("when the node does not exist on disk", func() {
		It("returns NotFound", func() {
			info := storage.UploadInfo{NodeExisted: false, Size: 100}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-1", info)
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsNotFound)
			Expect(ok).To(BeTrue(), "expected errtypes.NotFound, got %T: %v", err, err)
		})
	})

	Context("new file (NodeExisted=false)", func() {
		JustBeforeEach(touchTarget)

		It("writes xattrs and returns VersionCreated=false", func() {
			info := storage.UploadInfo{
				NodeExisted: false,
				Size:        42,
				Checksums: storage.UploadChecksums{
					SHA1:    []byte("sha1val"),
					MD5:     []byte("md5val"),
					Adler32: []byte("adler32val"),
				},
			}

			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-new", info)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.VersionCreated).To(BeFalse())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			blobID, err := n.Xattr(env.Ctx, prefixes.BlobIDAttr)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(blobID)).To(Equal("session-new"))
		})
	})

	Context("overwrite with versioning enabled (default)", func() {
		JustBeforeEach(touchTarget)

		It("creates a version file and returns VersionCreated=true", func() {
			// First PrepareUpload establishes initial metadata on the node.
			info1 := storage.UploadInfo{
				NodeExisted: false,
				Size:        10,
			}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-1", info1)
			Expect(err).ToNot(HaveOccurred())

			// Second call: overwrite.
			info2 := storage.UploadInfo{
				NodeExisted: true,
				Size:        20,
			}
			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-2", info2)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.VersionCreated).To(BeTrue())

			revisions, err := env.Fs.ListRevisions(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       ref.Path,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(revisions)).To(BeNumerically(">=", 1))
		})
	})

	Context("overwrite with versioning disabled", func() {
		JustBeforeEach(func() {
			var err error
			env, err = helpers.NewTestEnv(map[string]interface{}{
				"disable_versioning": true,
			})
			Expect(err).ToNot(HaveOccurred())

			ref = &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/upload-target.txt",
			}

			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					InitiateFileUpload: true,
					Stat:               true,
					ListFileVersions:   true,
				}, nil)
			_, err = env.Fs.TouchFile(env.Ctx, ref, false, "")
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not create a version file and returns VersionCreated=false", func() {
			// Lay down initial metadata.
			info1 := storage.UploadInfo{NodeExisted: false, Size: 10}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-1", info1)
			Expect(err).ToNot(HaveOccurred())

			// Overwrite.
			info2 := storage.UploadInfo{NodeExisted: true, Size: 20}
			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-2", info2)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.VersionCreated).To(BeFalse())

			revisions, err := env.Fs.ListRevisions(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       ref.Path,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(revisions).To(BeEmpty())
		})
	})

	// The posix driver builds its own aspects and disables versioning only there,
	// never in the config. Reading just the config sent it down the versioning path
	// it is built to skip, which failed the upload outright.
	Context("overwrite with versioning disabled through the aspects", func() {
		JustBeforeEach(func() {
			var err error
			env, err = helpers.NewTestEnv(nil, func(a *aspects.Aspects) {
				a.DisableVersioning = true
			})
			Expect(err).ToNot(HaveOccurred())

			ref = &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/upload-target.txt",
			}

			env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).
				Return(&provider.ResourcePermissions{
					InitiateFileUpload: true,
					Stat:               true,
					ListFileVersions:   true,
				}, nil)
			_, err = env.Fs.TouchFile(env.Ctx, ref, false, "")
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not create a version file and returns VersionCreated=false", func() {
			info1 := storage.UploadInfo{NodeExisted: false, Size: 10}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-1", info1)
			Expect(err).ToNot(HaveOccurred())

			info2 := storage.UploadInfo{NodeExisted: true, Size: 20}
			result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-2", info2)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.VersionCreated).To(BeFalse())

			revisions, err := env.Fs.ListRevisions(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       ref.Path,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(revisions).To(BeEmpty())
		})
	})

	Context("quota exceeded on overwrite", func() {
		JustBeforeEach(touchTarget)

		It("returns an error when the new size would exceed quota", func() {
			// Lay down initial metadata.
			info1 := storage.UploadInfo{NodeExisted: false, Size: 5}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-1", info1)
			Expect(err).ToNot(HaveOccurred())

			original := node.CheckQuota
			node.CheckQuota = func(_ context.Context, _ *node.Node, _ bool, _, _ uint64) (bool, error) {
				return false, errtypes.InsufficientStorage("quota exceeded")
			}
			defer func() { node.CheckQuota = original }()

			info := storage.UploadInfo{NodeExisted: true, Size: 20}
			_, err = env.Fs.PrepareUpload(env.Ctx, ref, "session-2", info)
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsInsufficientStorage)
			Expect(ok).To(BeTrue(), "expected errtypes.InsufficientStorage, got %T: %v", err, err)
		})
	})

	Context("quota exceeded on a new file", func() {
		JustBeforeEach(touchTarget)

		It("returns an error and reports the upload as an addition", func() {
			var (
				called       bool
				gotOverwrite bool
				gotOldSize   uint64
			)
			original := node.CheckQuota
			node.CheckQuota = func(_ context.Context, _ *node.Node, overwrite bool, oldSize, _ uint64) (bool, error) {
				called, gotOverwrite, gotOldSize = true, overwrite, oldSize
				return false, errtypes.InsufficientStorage("quota exceeded")
			}
			defer func() { node.CheckQuota = original }()

			info := storage.UploadInfo{NodeExisted: false, Size: 20}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-new", info)

			Expect(called).To(BeTrue(), "quota was not checked for a new file")
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsInsufficientStorage)
			Expect(ok).To(BeTrue(), "expected errtypes.InsufficientStorage, got %T: %v", err, err)

			// there are no bytes to replace, so the size must count as pure growth
			Expect(gotOverwrite).To(BeFalse())
			Expect(gotOldSize).To(BeZero())
		})
	})

	Context("precondition checks on overwrite", func() {
		var (
			oldEtag string
			oldTime time.Time
		)

		JustBeforeEach(func() {
			touchTarget()

			// Lay down initial metadata so the node has a valid etag/mtime.
			info1 := storage.UploadInfo{NodeExisted: false, Size: 5}
			_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-init", info1)
			Expect(err).ToNot(HaveOccurred())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			oldTime, err = n.GetMTime(env.Ctx)
			Expect(err).ToNot(HaveOccurred())
			oldEtag, err = node.CalculateEtag(n.ID, oldTime)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("IfMatch mismatch", func() {
			It("returns Aborted", func() {
				info := storage.UploadInfo{
					NodeExisted: true,
					Size:        5,
					IfMatch:     "wrong-etag",
				}
				_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-x", info)
				Expect(err).To(HaveOccurred())
				_, ok := err.(errtypes.IsAborted)
				Expect(ok).To(BeTrue(), "expected errtypes.Aborted, got %T: %v", err, err)
			})
		})

		Context("IfMatch match", func() {
			It("proceeds normally", func() {
				info := storage.UploadInfo{
					NodeExisted: true,
					Size:        5,
					IfMatch:     oldEtag,
				}
				result, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-match", info)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
			})
		})

		Context("IfNoneMatch=* on existing node", func() {
			It("returns Aborted", func() {
				info := storage.UploadInfo{
					NodeExisted: true,
					Size:        5,
					IfNoneMatch: "*",
				}
				_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-x", info)
				Expect(err).To(HaveOccurred())
				_, ok := err.(errtypes.IsAborted)
				Expect(ok).To(BeTrue(), "expected errtypes.Aborted, got %T: %v", err, err)
			})
		})

		Context("IfUnmodifiedSince violated", func() {
			It("returns Aborted when the node was modified after the given time", func() {
				before := oldTime.Add(-time.Second)
				info := storage.UploadInfo{
					NodeExisted:       true,
					Size:              5,
					IfUnmodifiedSince: before,
				}
				_, err := env.Fs.PrepareUpload(env.Ctx, ref, "session-x", info)
				Expect(err).To(HaveOccurred())
				_, ok := err.(errtypes.IsAborted)
				Expect(ok).To(BeTrue(), "expected errtypes.Aborted, got %T: %v", err, err)
			})
		})
	})
})
