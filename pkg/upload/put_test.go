package upload

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"hash/adler32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/utils"
)

var _ = Describe("Upload", func() {
	var (
		ctx         context.Context
		store       *FileStore
		fs          *fakeFS
		c           *coordinator
		chunkFolder string
	)

	// initiated primes a session the way a PUT arrives to it, no bytes written yet.
	initiated := func(chunk string) Session {
		session := store.New(ctx)
		session.SetMetadata("providerID", mountID)
		session.SetMetadata("filename", "report.docx")
		session.SetMetadata("mtime", utils.TimeToOCMtime(utils.TSToTime(utils.TSNow())))
		session.SetStorageValue("NodeName", "report.docx")
		session.SetStorageValue("Dir", "/Shares/project")
		session.SetStorageValue("SpaceRoot", spaceRoot)
		session.SetStorageValue("NodeParentId", parentID)
		session.SetStorageValue("NodeId", nodeID)
		session.SetExecutant(&userpb.User{
			Id:       &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
			Username: "alice",
		})
		session.SetSize(bodyLen)
		if chunk != "" {
			session.SetStorageValue("Chunk", chunk)
		}
		Expect(session.TouchBin()).To(Succeed())
		Expect(session.Persist(ctx)).To(Succeed())
		return session
	}

	// put is the call the dataprovider makes: the session id rides in the ref path.
	put := func(session Session, content string) (*provider.ResourceInfo, error) {
		return c.Upload(ctx, Request{
			Ref:    &provider.Reference{Path: "/" + session.ID()},
			Body:   io.NopCloser(strings.NewReader(content)),
			Length: int64(len(content)),
		}, nil)
	}

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
		})
		store = NewFileStore(filepath.Join(GinkgoT().TempDir(), "uploads"), TokenOptions{}, nopLog())
		Expect(store.Setup()).To(Succeed())
		// The deployment provisions this folder, on storage shared across pods.
		chunkFolder = filepath.Join(GinkgoT().TempDir(), "chunks")
		Expect(os.MkdirAll(chunkFolder, 0700)).To(Succeed())

		fs = &fakeFS{
			touched: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			},
		}
		c = NewCoordinator(fs, store, "", nil)
	})

	It("stages the body and commits it", func() {
		session := initiated("")

		ri, err := put(session, body)

		Expect(err).ToNot(HaveOccurred())
		Expect(ri.GetEtag()).To(Equal("etag-after-commit"))
		Expect(fs.calls).To(ContainElement("CommitUpload(length=17)"))
	})

	It("reports an unknown session id as not found", func() {
		_, err := c.Upload(ctx, Request{
			Ref:    &provider.Reference{Path: "/no-such-session"},
			Body:   io.NopCloser(strings.NewReader(body)),
			Length: bodyLen,
		}, nil)

		Expect(err).To(HaveOccurred())
		Expect(fs.calls).To(BeEmpty())
	})

	// A truncated body would otherwise be committed as if complete.
	It("rejects a body shorter than the declared length", func() {
		session := initiated("")

		_, err := c.Upload(ctx, Request{
			Ref:    &provider.Reference{Path: "/" + session.ID()},
			Body:   io.NopCloser(strings.NewReader("short")),
			Length: bodyLen,
		}, nil)

		Expect(err).To(BeAssignableToTypeOf(errtypes.PartialContent("")))
		// Nothing reached the driver, so the session can still be retried.
		Expect(fs.calls).To(BeEmpty())
	})

	// A session whose staged file is gone cannot be resumed.
	It("reports a session whose staged file is gone as not found", func() {
		session := initiated("")
		Expect(os.Remove(session.BinPath())).To(Succeed())

		_, err := put(session, body)

		Expect(err).To(HaveOccurred())
		Expect(fs.calls).To(BeEmpty())
	})

	// The connection dropped mid-body, so the staged bytes are not the whole file.
	It("propagates a failure to read the body", func() {
		session := initiated("")

		_, err := c.Upload(ctx, Request{
			Ref:    &provider.Reference{Path: "/" + session.ID()},
			Body:   io.NopCloser(failingReader{err: errors.New("connection reset by peer")}),
			Length: bodyLen,
		}, nil)

		Expect(err).To(MatchError("connection reset by peer"))
		Expect(fs.calls).To(BeEmpty())
	})

	It("propagates a failure from the finish path", func() {
		session := initiated("")
		fs.commitErr = errors.New("blobstore unavailable")

		_, err := put(session, body)

		Expect(err).To(MatchError("blobstore unavailable"))
	})

	// The events middleware announces the upload from this callback.
	Describe("the finished callback", func() {
		It("names the file that was written", func() {
			session := initiated("")
			var gotRef *provider.Reference
			var gotExecutant *userpb.UserId

			_, err := c.Upload(ctx, Request{
				Ref:    &provider.Reference{Path: "/" + session.ID()},
				Body:   io.NopCloser(strings.NewReader(body)),
				Length: bodyLen,
			}, func(_, executant *userpb.UserId, ref *provider.Reference) {
				gotExecutant, gotRef = executant, ref
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(gotExecutant.GetOpaqueId()).To(Equal("alice"))
			Expect(gotRef.GetPath()).To(Equal("./Shares/project/report.docx"))
			Expect(gotRef.GetResourceId().GetSpaceId()).To(Equal(spaceRoot))
		})

		It("is not called when the upload failed", func() {
			session := initiated("")
			fs.commitErr = errors.New("blobstore unavailable")
			called := false

			_, err := c.Upload(ctx, Request{
				Ref:    &provider.Reference{Path: "/" + session.ID()},
				Body:   io.NopCloser(strings.NewReader(body)),
				Length: bodyLen,
			}, func(_, _ *userpb.UserId, _ *provider.Reference) { called = true })

			Expect(err).To(HaveOccurred())
			Expect(called).To(BeFalse())
		})
	})

	// Legacy chunking v1: one PUT per chunk, each its own session.
	Describe("legacy chunking", func() {
		BeforeEach(func() {
			c = NewCoordinator(fs, store, chunkFolder, nil)
		})

		It("reports a partial upload until the last chunk arrives", func() {
			session := initiated("report.docx-chunking-abc-2-0")

			_, err := put(session, "hello ")

			Expect(err).To(BeAssignableToTypeOf(errtypes.PartialContent("")))
			Expect(fs.calls).To(BeEmpty())
		})

		// The bytes live in the chunk folder, not in this session.
		It("discards the session of an intermediate chunk", func() {
			session := initiated("report.docx-chunking-abc-2-0")

			_, err := put(session, "hello ")
			Expect(err).To(HaveOccurred())

			_, sErr := os.Stat(session.BinPath())
			Expect(errors.Is(sErr, os.ErrNotExist)).To(BeTrue())
		})

		// The declared length covers only the final chunk, not the assembled file.
		It("commits the assembled file once the last chunk arrives", func() {
			first := initiated("report.docx-chunking-abc-2-0")
			_, err := put(first, "hello ")
			Expect(err).To(HaveOccurred())

			last := initiated("report.docx-chunking-abc-2-1")
			ri, err := put(last, "coordinator")

			Expect(err).ToNot(HaveOccurred())
			Expect(ri.GetEtag()).To(Equal("etag-after-commit"))
			Expect(fs.calls).To(ContainElement("CommitUpload(length=17)"))
			Expect(fs.committed.Length).To(Equal(bodyLen))
		})

		// The client may send the chunks in any order.
		It("assembles the chunks in index order, not arrival order", func() {
			last := initiated("report.docx-chunking-abc-2-1")
			_, err := put(last, "coordinator")
			Expect(err).To(HaveOccurred())

			first := initiated("report.docx-chunking-abc-2-0")
			_, err = put(first, "hello ")

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.committed.Length).To(Equal(bodyLen))
		})

		// Without a chunk folder the earlier chunks would be silently lost.
		It("refuses when no chunk folder is configured", func() {
			c = NewCoordinator(fs, store, "", nil)
			session := initiated("report.docx-chunking-abc-2-0")

			_, err := put(session, body)

			Expect(err).To(BeAssignableToTypeOf(errtypes.NotSupported("")))
			Expect(fs.calls).To(BeEmpty())
		})

		It("propagates a failure to accumulate the chunk", func() {
			Expect(os.RemoveAll(chunkFolder)).To(Succeed())
			session := initiated("report.docx-chunking-abc-2-0")

			_, err := put(session, "hello ")

			Expect(err).To(HaveOccurred())
			Expect(fs.calls).To(BeEmpty())
		})

		// It is the assembled size that has to match what was staged.
		It("propagates a failure to stage the assembled file", func() {
			first := initiated("report.docx-chunking-abc-2-0")
			_, err := put(first, "hello ")
			Expect(err).To(HaveOccurred())

			last := initiated("report.docx-chunking-abc-2-1")
			Expect(os.Remove(last.BinPath())).To(Succeed())

			_, err = put(last, "coordinator")

			Expect(err).To(HaveOccurred())
			Expect(fs.calls).To(BeEmpty())
		})
	})
})

// failingReader stands in for a body that cannot be read to the end.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

var _ = Describe("the finish path", func() {
	var (
		ctx   context.Context
		store *FileStore
		fs    *fakeFS
		c     *coordinator
	)

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
		})
		store = NewFileStore(filepath.Join(GinkgoT().TempDir(), "uploads"), TokenOptions{}, nopLog())
		Expect(store.Setup()).To(Succeed())

		fs = &fakeFS{
			touched: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			},
		}
		c = NewCoordinator(fs, store, "", nil)
	})

	Describe("creating the node", func() {
		// The parent went away while the bytes were in flight.
		It("reports a vanished parent as a precondition failure", func() {
			session := stagedSession(ctx, store, false)
			fs.touchErr = errtypes.NotFound("parent-1")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(BeAssignableToTypeOf(errtypes.PreconditionFailed("")))
			Expect(fs.calls).To(Equal([]string{"TouchFile(markprocessing=false)"}))
		})

		It("propagates any other failure to create the node", func() {
			session := stagedSession(ctx, store, false)
			fs.touchErr = errors.New("metadata backend down")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("metadata backend down"))
		})

		It("discards the staged bytes when the node cannot be created", func() {
			session := stagedSession(ctx, store, false)
			fs.touchErr = errors.New("metadata backend down")

			_, err := c.finishUpload(ctx, session)
			Expect(err).To(HaveOccurred())

			_, sErr := os.Stat(session.BinPath())
			Expect(errors.Is(sErr, os.ErrNotExist)).To(BeTrue())
		})

		// The node has no id of its own until it is created.
		It("addresses the node as a child of its parent", func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.touchRef.GetPath()).To(Equal("report.docx"))
			Expect(fs.touchRef.GetResourceId().GetOpaqueId()).To(Equal(parentID))
			Expect(fs.touchRef.GetResourceId().GetSpaceId()).To(Equal(spaceID))
		})

		// The mtime is what the desktop client asked the file to keep.
		It("stamps the mtime the client asked for", func() {
			session := stagedSession(ctx, store, false)
			mtime := utils.TimeToOCMtime(time.Unix(1700000000, 0))
			session.SetMetadata("mtime", mtime)
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.touchMtime).To(Equal(mtime))
		})

		// The commit may run in another process, and reads it back off the session.
		It("records the space owner the driver reported", func() {
			session := stagedSession(ctx, store, false)
			fs.touchedOwner = &userpb.UserId{
				OpaqueId: "owner-1",
				Idp:      "idp.example.com",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			}

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(session.SpaceOwner().GetOpaqueId()).To(Equal("owner-1"))
			Expect(session.SpaceOwner().GetIdp()).To(Equal("idp.example.com"))
		})
	})

	Describe("the checksum the client announced", func() {
		checksumOf := func(algorithm string) string {
			switch algorithm {
			case "sha1":
				h := sha1.Sum([]byte(body)) //nolint:gosec
				return "sha1 " + hex.EncodeToString(h[:])
			case "md5":
				h := md5.Sum([]byte(body)) //nolint:gosec
				return "md5 " + hex.EncodeToString(h[:])
			default:
				h := adler32.New()
				_, _ = h.Write([]byte(body))
				return "adler32 " + hex.EncodeToString(h.Sum(nil))
			}
		}

		DescribeTable("is verified against the staged bytes",
			func(algorithm string) {
				session := stagedSession(ctx, store, true)
				session.SetMetadata("checksum", checksumOf(algorithm))
				Expect(session.Persist(ctx)).To(Succeed())

				_, err := c.finishUpload(ctx, session)

				Expect(err).ToNot(HaveOccurred())
				Expect(fs.calls).To(ContainElement("CommitUpload(length=17)"))
			},
			Entry("sha1", "sha1"),
			Entry("md5", "md5"),
			Entry("adler32", "adler32"),
		)

		DescribeTable("is rejected when it does not match",
			func(algorithm string) {
				session := stagedSession(ctx, store, true)
				session.SetMetadata("checksum", algorithm+" "+strings.Repeat("0", 40))
				Expect(session.Persist(ctx)).To(Succeed())

				_, err := c.finishUpload(ctx, session)

				Expect(err).To(HaveOccurred())
				Expect(fs.calls).ToNot(ContainElement(ContainSubstring("CommitUpload")))
			},
			Entry("sha1", "sha1"),
			Entry("md5", "md5"),
			Entry("adler32", "adler32"),
		)

		// The algorithm was already checked at initiate.
		It("is rejected when the algorithm is unknown", func() {
			session := stagedSession(ctx, store, true)
			session.SetMetadata("checksum", "sha256 abcdef")
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})

		It("is rejected when it carries no algorithm", func() {
			session := stagedSession(ctx, store, true)
			session.SetMetadata("checksum", "abcdef")
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})

		It("cannot be verified once the staged bytes are gone", func() {
			session := stagedSession(ctx, store, true)
			Expect(os.Remove(session.BinPath())).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(HaveOccurred())
		})

		// The announced checksum is not in Metadata(), only in the raw session.
		It("cannot be verified when the session cannot be read back", func() {
			session := &brokenSession{
				Session:    stagedSession(ctx, store, true),
				getInfoErr: errors.New("session file unreadable"),
			}

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("session file unreadable"))
			Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
		})

		// Computed once here, so the driver does not have to re-read the staged file.
		It("is stored for the commit to pick up", func() {
			session := stagedSession(ctx, store, true)

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			expected := sha1.Sum([]byte(body)) //nolint:gosec
			Expect(session.Checksums().SHA1).To(Equal(expected[:]))
		})
	})

	// The resource may have changed while the bytes were being uploaded.
	Describe("what the driver is told about the upload", func() {
		It("carries the conditional headers the client sent", func() {
			session := stagedSession(ctx, store, true)
			session.SetMetadata("if-match", "etag-1")
			session.SetMetadata("if-unmodified-since", "2026-08-13T10:00:00Z")
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.prepareInfo.IfMatch).To(Equal("etag-1"))
			Expect(fs.prepareInfo.IfUnmodifiedSince.UTC().Format("2006-01-02T15:04:05Z")).To(Equal("2026-08-13T10:00:00Z"))
			Expect(fs.prepareInfo.NodeExisted).To(BeTrue())
		})

		It("rejects an mtime it cannot parse", func() {
			session := stagedSession(ctx, store, true)
			session.SetMetadata("mtime", "not-a-time")
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
			Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
		})

		It("rejects an if-unmodified-since it cannot parse", func() {
			session := stagedSession(ctx, store, true)
			session.SetMetadata("if-unmodified-since", "not-a-time")
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(BeAssignableToTypeOf(errtypes.BadRequest("")))
		})
	})

	Describe("committing", func() {
		It("cannot commit once the staged bytes are gone", func() {
			session := stagedSession(ctx, store, true)
			fs.afterPrepare = func() { Expect(os.Remove(session.BinPath())).To(Succeed()) }

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(HaveOccurred())
			Expect(fs.calls).To(ContainElement("RollbackUpload(nodeExisted=true,sizeDiff=0)"))
		})

		// The bytes are already committed, and the cleanup job resolves the flag.
		It("succeeds even when the node cannot be unmarked", func() {
			session := stagedSession(ctx, store, true)
			fs.markErrAfter = 1
			fs.markErr = errors.New("flock timeout")

			ri, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(ri.GetEtag()).To(Equal("etag-after-commit"))
		})

		// A write-only share cannot stat what it just uploaded.
		It("falls back to what the session knows when the result cannot be read", func() {
			session := stagedSession(ctx, store, true)
			fs.mdErr = errtypes.PermissionDenied("report.docx")

			ri, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(ri.GetId().GetOpaqueId()).To(Equal(nodeID))
			Expect(ri.GetSize()).To(Equal(uint64(bodyLen)))
			Expect(ri.GetMtime()).ToNot(BeNil())
		})

		// Drivers do not know their own mount id, and an unstamped id will not resolve.
		It("stamps the mount id the driver left out", func() {
			session := stagedSession(ctx, store, true)
			fs.md = &provider.ResourceInfo{
				Id:   &provider.ResourceId{SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			}

			ri, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(ri.GetId().GetStorageId()).To(Equal(mountID))
		})
	})

	// An upload whose session cannot be written can never be finished.
	Describe("when the session cannot be persisted", func() {
		// The node id TouchFile returned is what would be lost.
		It("rolls the mark back", func() {
			session := &brokenSession{Session: stagedSession(ctx, store, false), failPersist: true}

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("no space left on device"))
			Expect(fs.calls).To(Equal([]string{
				"TouchFile(markprocessing=false)",
				"MarkProcessing(true)",
				"RollbackUpload(nodeExisted=false,sizeDiff=0)",
				"MarkProcessing(false)",
			}))
		})

		// The size PrepareUpload propagated is what would be lost.
		It("rolls the prepared upload back", func() {
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}
			// The mark's own persist has to get through for the prepare to be reached.
			session := &brokenSession{
				Session:          stagedSession(ctx, store, true),
				failPersist:      true,
				failPersistAfter: 1,
			}

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("no space left on device"))
			Expect(fs.calls).To(Equal([]string{
				"MarkProcessing(true)",
				"PrepareUpload(size=17)",
				"RollbackUpload(nodeExisted=true,sizeDiff=17)",
				"MarkProcessing(false)",
			}))
		})
	})

	// Cleanup runs on paths that are already failing, so it can only be logged.
	Describe("when the cleanup itself fails", func() {
		It("still reports the original failure", func() {
			session := stagedSession(ctx, store, true)
			fs.commitErr = errors.New("blobstore unavailable")
			fs.rollbackErr = errors.New("no such node")
			fs.markErrAfter = 1
			fs.markErr = errors.New("flock timeout")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("blobstore unavailable"))
		})

		// An Uploader-only role may leave the empty file behind.
		It("still reports the failed mark when the node cannot be deleted", func() {
			session := stagedSession(ctx, store, false)
			fs.markErr = errors.New("flock timeout")
			fs.deleteErr = errtypes.PermissionDenied("report.docx")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("flock timeout"))
			Expect(fs.calls).To(ContainElement("Delete"))
		})

		// Nothing was prepared here, so the node is purged rather than reverted.
		It("still reports the original failure when the purge fails", func() {
			session := stagedSession(ctx, store, false)
			fs.prepareErr = errors.New("precondition failed")
			fs.rollbackErr = errors.New("no such node")
			fs.markErrAfter = 1
			fs.markErr = errors.New("flock timeout")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("precondition failed"))
			Expect(fs.calls).To(Equal([]string{
				"TouchFile(markprocessing=false)",
				"MarkProcessing(true)",
				"PrepareUpload(size=17)",
				"RollbackUpload(nodeExisted=false,sizeDiff=0)",
				"MarkProcessing(false)",
			}))
		})
	})
})
