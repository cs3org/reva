package upload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/utils"
)

var _ = Describe("InitiateUpload", func() {
	var (
		ctx    context.Context
		store  *FileStore
		fs     *fakeFS
		c      *coordinator
		newRef *provider.Reference
	)

	// uploadable is a directory Alice may upload into.
	uploadable := func(id string) *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Id:            &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: id},
			Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			PermissionSet: &provider.ResourcePermissions{InitiateFileUpload: true},
		}
	}

	// existingFile is the file an overwrite targets.
	existingFile := func() *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Id:            &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			ParentId:      &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: parentID},
			Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
			Name:          "report.docx",
			Path:          "/Shares/project/report.docx",
			Size:          10,
			PermissionSet: &provider.ResourcePermissions{InitiateFileUpload: true},
			Owner:         &userpb.UserId{OpaqueId: "owner-1", Idp: "idp.example.com"},
		}
	}

	// session reads back the single session InitiateUpload persisted.
	session := func(ids map[string]string) Session {
		s, err := store.Get(ctx, ids["simple"])
		Expect(err).ToNot(HaveOccurred())
		return s
	}

	// rawMetadata is everything the session stored, not just what Metadata() exposes.
	rawMetadata := func(ids map[string]string) map[string]string {
		info, err := session(ids).GetInfo(ctx)
		Expect(err).ToNot(HaveOccurred())
		return info.MetaData
	}

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id:       &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
			Username: "alice",
		})
		store = NewFileStore(filepath.Join(GinkgoT().TempDir(), "uploads"), TokenOptions{}, nopLog())
		Expect(store.Setup()).To(Succeed())

		fs = &fakeFS{
			quota:   1 << 30,
			touched: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			},
			// The target does not exist, its parent does.
			tree: map[string]*provider.ResourceInfo{
				"/Shares/project": uploadable(parentID),
				"/Shares":         uploadable("shares-1"),
			},
		}
		c = NewCoordinator(fs, store, "", nil)
		newRef = &provider.Reference{
			ResourceId: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID},
			Path:       "/Shares/project/report.docx",
		}
	})

	Context("for a new file", func() {
		It("returns an id for both protocols", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(ids).To(HaveKey("simple"))
			Expect(ids["tus"]).To(Equal(ids["simple"]))
			Expect(ids["simple"]).ToNot(BeEmpty())
		})

		It("resolves the target from the parent directory", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)
			Expect(err).ToNot(HaveOccurred())

			s := session(ids)
			Expect(s.Filename()).To(Equal("report.docx"))
			Expect(s.Dir()).To(Equal("/Shares/project"))
			Expect(s.NodeParentID()).To(Equal(parentID))
			Expect(s.SpaceID()).To(Equal(spaceID))
			Expect(s.Size()).To(Equal(bodyLen))
		})

		// finishUpload replaces the placeholder with the id TouchFile returns.
		It("mints a placeholder node id and does not claim the node exists", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)
			Expect(err).ToNot(HaveOccurred())

			s := session(ids)
			Expect(s.NodeExists()).To(BeFalse())
			Expect(s.NodeID()).ToNot(BeEmpty())
			Expect(s.NodeID()).ToNot(Equal(nodeID))
		})

		// The HTTP context is gone by the time the bytes arrive.
		It("records the executant and the lock the request carried", func() {
			ctx = ctxpkg.ContextSetLockID(ctx, "lock-1")

			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(session(ids).ExecutantUser().GetUsername()).To(Equal("alice"))
			Expect(rawMetadata(ids)["lockid"]).To(Equal("lock-1"))
		})

		It("stages an empty bin file for the bytes to be appended to", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)
			Expect(err).ToNot(HaveOccurred())

			info, err := session(ids).GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.Offset).To(BeZero())
			Expect(info.Size).To(Equal(bodyLen))
		})

		It("reaches no write method on the driver", func() {
			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(Equal([]string{
				"GetMD(/Shares/project/report.docx)",
				"GetQuota",
				"GetMD(/Shares/project)",
			}))
		})
	})

	Context("for an overwrite", func() {
		BeforeEach(func() {
			fs.tree["/Shares/project/report.docx"] = existingFile()
		})

		It("resolves the target from the file itself", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)
			Expect(err).ToNot(HaveOccurred())

			s := session(ids)
			Expect(s.NodeExists()).To(BeTrue())
			Expect(s.NodeID()).To(Equal(nodeID))
			Expect(s.NodeParentID()).To(Equal(parentID))
			Expect(s.Dir()).To(Equal("/Shares/project"))
			Expect(s.SpaceOwner().GetOpaqueId()).To(Equal("owner-1"))
		})

		It("does not look up the parent", func() {
			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(Equal([]string{
				"GetMD(/Shares/project/report.docx)",
				"GetQuota",
				"GetLock",
			}))
		})

		// GetMD returns only the basename for an id-based ref.
		It("asks for the full path when the ref is id-based", func() {
			fs.tree["."] = existingFile()
			fs.pathByID = "/Shares/project/report.docx"

			ids, err := c.InitiateUpload(ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Path:       ".",
			}, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(ContainElement("GetPathByID"))
			Expect(session(ids).Dir()).To(Equal("/Shares/project"))
		})

		It("refuses when the caller may not upload", func() {
			existing := existingFile()
			existing.PermissionSet = &provider.ResourcePermissions{Stat: true}
			fs.tree["/Shares/project/report.docx"] = existing

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PermissionDenied("")))
		})

		It("refuses to overwrite a directory", func() {
			fs.tree["/Shares/project/report.docx"] = uploadable("dir-1")

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PreconditionFailed("")))
		})

		// if-none-match: * means "only create", so an existing file is a conflict.
		It("refuses when the client asked to create only", func() {
			_, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"if-none-match": "*"})

			Expect(err).To(BeAssignableToTypeOf(errtypes.Aborted("")))
		})
	})

	Context("when the target is locked", func() {
		BeforeEach(func() {
			fs.tree["/Shares/project/report.docx"] = existingFile()
		})

		It("refuses a caller holding no lock", func() {
			fs.lock = &provider.Lock{LockId: "lock-1"}

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.Locked("lock-1")))
		})

		It("refuses a caller holding the wrong lock", func() {
			fs.lock = &provider.Lock{LockId: "lock-1"}

			_, err := c.InitiateUpload(ctxpkg.ContextSetLockID(ctx, "lock-2"), newRef, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.Aborted("mismatching lock")))
		})

		It("accepts a caller holding the right lock", func() {
			fs.lock = &provider.Lock{LockId: "lock-1"}

			_, err := c.InitiateUpload(ctxpkg.ContextSetLockID(ctx, "lock-1"), newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
		})

		// The file carries no lock, so a client that thinks it holds one is out of date.
		It("refuses a lock the file does not carry", func() {
			fs.lockErr = errtypes.NotFound("no lock")

			_, err := c.InitiateUpload(ctxpkg.ContextSetLockID(ctx, "lock-1"), newRef, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.Aborted("not locked")))
		})

		// Drivers without lock support error the same way an unlocked file does.
		It("proceeds when the driver cannot report locks", func() {
			fs.lockErr = errtypes.NotSupported("locks")

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when the parent directory is not there", func() {
		// A visible ancestor means the directory really is missing.
		It("reports a precondition failure when an ancestor is visible", func() {
			delete(fs.tree, "/Shares/project")

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PreconditionFailed("")))
			Expect(fs.calls).To(ContainElement("GetMD(/Shares)"))
		})

		// No visible ancestor must not reveal whether the directory exists.
		It("reports permission denied when no ancestor is visible", func() {
			fs.tree = map[string]*provider.ResourceInfo{}

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PermissionDenied("")))
		})

		It("propagates a failure that is neither success nor not-found", func() {
			fs.mdErrs = map[string]error{"/Shares/project": errors.New("metadata backend down")}

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError("metadata backend down"))
		})

		It("refuses when the caller may not upload into the parent", func() {
			fs.tree["/Shares/project"] = &provider.ResourceInfo{
				Id:            &provider.ResourceId{SpaceId: spaceID, OpaqueId: parentID},
				PermissionSet: &provider.ResourcePermissions{Stat: true},
			}

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PermissionDenied("")))
		})
	})

	Context("quota", func() {
		It("refuses an upload larger than what is left", func() {
			fs.quota = 5

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.InsufficientStorage("quota exceeded")))
		})

		// An overwrite only adds the difference, so it fits where a new file would not.
		It("counts only the bytes an overwrite adds", func() {
			fs.tree["/Shares/project/report.docx"] = existingFile()
			fs.quota = 8

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
		})

		// An unreadable quota is treated as no limit.
		It("proceeds when the quota cannot be read", func() {
			fs.quotaErr = errors.New("no quota support")

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
		})

		// A deferred size is not known yet, so there is nothing to check it against.
		It("skips the check for an upload of unknown size", func() {
			fs.quota = 0

			_, err := c.InitiateUpload(ctx, newRef, -1, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).ToNot(ContainElement("GetQuota"))
		})
	})

	Context("request metadata", func() {
		It("stores the mtime the client asked for", func() {
			mtime := utils.TimeToOCMtime(time.Unix(1700000000, 0))

			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"mtime": mtime})

			Expect(err).ToNot(HaveOccurred())
			Expect(session(ids).Metadata()["mtime"]).To(Equal(mtime))
		})

		It("falls back to now when the client sent no usable mtime", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"mtime": "null"})

			Expect(err).ToNot(HaveOccurred())
			stored, mErr := utils.MTimeToTime(session(ids).Metadata()["mtime"])
			Expect(mErr).ToNot(HaveOccurred())
			Expect(stored).To(BeTemporally("~", time.Now(), time.Minute))
		})

		// The preconditions are only evaluated once the bytes have arrived.
		It("records the conditional headers for the finish path", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{
				"if-match":            "etag-1",
				"if-unmodified-since": "2026-08-13T10:00:00Z",
				"expires":             utils.TimeToOCMtime(time.Unix(1700000000, 0)),
			})

			Expect(err).ToNot(HaveOccurred())
			md := session(ids).Metadata()
			Expect(md["if-match"]).To(Equal("etag-1"))
			Expect(md["if-unmodified-since"]).To(Equal("2026-08-13T10:00:00Z"))
			Expect(session(ids).Expires()).To(BeTemporally("==", time.Unix(1700000000, 0)))
		})

		// A third-party copy sends no Upload-Length.
		It("marks the size as deferred when the client cannot state it", func() {
			ids, err := c.InitiateUpload(ctx, newRef, 0, map[string]string{"sizedeferred": "true"})

			Expect(err).ToNot(HaveOccurred())
			info, iErr := store.Get(ctx, ids["simple"])
			Expect(iErr).To(HaveOccurred()) // finished by the zero-length path
			Expect(info).To(BeNil())
		})

		It("stores a checksum the driver can verify against", func() {
			ids, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"checksum": "sha1 abcdef"})

			Expect(err).ToNot(HaveOccurred())
			Expect(rawMetadata(ids)["checksum"]).To(Equal("sha1 abcdef"))
		})

		It("rejects a checksum without an algorithm", func() {
			_, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"checksum": "abcdef"})

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})

		It("rejects an algorithm the drivers cannot compute", func() {
			_, err := c.InitiateUpload(ctx, newRef, bodyLen, map[string]string{"checksum": "sha256 abcdef"})

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})
	})

	// A zero-length upload has no bytes to append, so there is nothing to wait for.
	Context("for a zero-length upload", func() {
		It("commits it straight away", func() {
			_, err := c.InitiateUpload(ctx, newRef, 0, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(ContainElement("CommitUpload(length=0)"))
		})

		It("reports the failure to the client", func() {
			fs.commitErr = errors.New("blobstore unavailable")

			_, err := c.InitiateUpload(ctx, newRef, 0, nil)

			Expect(err).To(MatchError("blobstore unavailable"))
		})
	})

	Context("for a legacy chunked upload", func() {
		It("resolves the real target behind the chunk path", func() {
			ids, err := c.InitiateUpload(ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID},
				Path:       "/Shares/project/report.docx-chunking-abc-3-0",
			}, bodyLen, nil)

			Expect(err).ToNot(HaveOccurred())
			s := session(ids)
			Expect(s.Filename()).To(Equal("report.docx"))
			Expect(s.Chunk()).To(Equal("report.docx-chunking-abc-3-0"))
			Expect(fs.calls[0]).To(Equal("GetMD(/Shares/project/report.docx)"))
		})

		It("rejects a chunk index the total does not allow", func() {
			_, err := c.InitiateUpload(ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID},
				Path:       "/Shares/project/report.docx-chunking-abc-3-7",
			}, bodyLen, nil)

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})
	})

	// An unwritten session would leave the client an id that resolves to nothing.
	Context("when the session cannot be created", func() {
		It("reports a bin file it could not create", func() {
			Expect(os.RemoveAll(store.uploadDir)).To(Succeed())

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError(ContainSubstring("could not create bin file")))
		})

		It("reports a session it could not persist, and stages nothing", func() {
			c = NewCoordinator(fs, &brokenStore{SessionStore: store, failPersist: true}, "", nil)

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError(ContainSubstring("could not persist session")))
			staged, gErr := filepath.Glob(filepath.Join(store.uploadDir, "*"))
			Expect(gErr).ToNot(HaveOccurred())
			Expect(staged).To(BeEmpty())
		})
	})

	Context("when the target cannot be resolved", func() {
		It("propagates an error that is neither success nor not-found", func() {
			fs.tree = nil
			fs.mdErr = errors.New("metadata backend down")

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError("metadata backend down"))
		})

		// A nameless target would otherwise only fail at commit, in another process.
		It("rejects a target the driver reported without a name", func() {
			nameless := existingFile()
			nameless.Name = ""
			fs.tree["/Shares/project/report.docx"] = nameless

			_, err := c.InitiateUpload(ctx, newRef, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.BadRequest("coordinator: missing filename in ref")))
		})

		It("rejects a parent whose path the driver reported as empty", func() {
			fs.tree["."] = uploadable(parentID)
			fs.pathByID = ""

			_, err := c.InitiateUpload(ctx, &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: parentID},
				Path:       "./report.docx",
			}, bodyLen, nil)

			Expect(err).To(MatchError(errtypes.BadRequest("coordinator: could not determine upload directory")))
		})
	})
})
