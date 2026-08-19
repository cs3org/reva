package upload

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
)

var _ = Describe("tusAdapter", func() {
	var (
		ctx   context.Context
		store *FileStore
		fs    *fakeFS
		c     *coordinator
	)

	// adapterFor wraps a session the way GetUpload does.
	adapterFor := func(session Session) *tusAdapter {
		return &tusAdapter{session: session, coord: c}
	}

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

	Describe("the data path", func() {
		It("reports the staged offset and the declared size", func() {
			session := stagedSession(ctx, store, false)

			info, err := adapterFor(session).GetInfo(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(info.Offset).To(Equal(bodyLen))
			Expect(info.Size).To(Equal(bodyLen))
			Expect(info.ID).To(Equal(session.ID()))
		})

		It("appends each chunk at the running offset", func() {
			session := store.New(ctx)
			Expect(session.TouchBin()).To(Succeed())
			up := adapterFor(session)

			first, err := up.WriteChunk(ctx, 0, strings.NewReader("hello "))
			Expect(err).ToNot(HaveOccurred())
			second, err := up.WriteChunk(ctx, first, strings.NewReader("coordinator"))
			Expect(err).ToNot(HaveOccurred())

			Expect(first + second).To(Equal(bodyLen))
			staged, err := os.ReadFile(session.BinPath())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(staged)).To(Equal(body))
		})

		It("reads back the bytes it staged", func() {
			session := stagedSession(ctx, store, false)

			reader, err := adapterFor(session).GetReader(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer reader.Close()

			staged, err := io.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(staged)).To(Equal(body))
		})
	})

	Describe("FinishUpload", func() {
		It("runs the coordinator's finish path", func() {
			session := stagedSession(ctx, store, false)

			Expect(adapterFor(session).FinishUpload(ctx)).To(Succeed())
			Expect(fs.calls).To(Equal([]string{
				"TouchFile(markprocessing=false)",
				"MarkProcessing(true)",
				"PrepareUpload(size=17)",
				"CommitUpload(length=17)",
				"MarkProcessing(false)",
				"GetMD()",
			}))
		})

		// tusd answers an error type it does not recognise with a bare 500.
		DescribeTable("translates the driver's error to an HTTP status",
			func(driverErr error, code string, status int) {
				session := stagedSession(ctx, store, true)
				fs.prepareErr = driverErr

				err := adapterFor(session).FinishUpload(ctx)

				var tusErr tusd.Error
				Expect(errors.As(err, &tusErr)).To(BeTrue())
				Expect(tusErr.ErrorCode).To(Equal(code))
				Expect(tusErr.HTTPResponse.StatusCode).To(Equal(status))
			},
			Entry("already exists", errtypes.AlreadyExists("report.docx"), "ERR_ALREADY_EXISTS", http.StatusConflict),
			Entry("still processing", errtypes.ResourceProcessing("report.docx"), "ERR_TOO_EARLY", http.StatusTooEarly),
			Entry("too early", errtypes.TooEarly("report.docx"), "ERR_TOO_EARLY", http.StatusTooEarly),
			Entry("aborted", errtypes.Aborted("mismatching lock"), "ERR_PRECONDITION_FAILED", http.StatusPreconditionFailed),
			Entry("precondition failed", errtypes.PreconditionFailed("resource is not a file"), "ERR_PRECONDITION_FAILED", http.StatusMethodNotAllowed),
			Entry("locked", errtypes.Locked("lock-1"), "ERR_LOCKED", http.StatusLocked),
			Entry("bad request", errtypes.BadRequest("invalid mtime"), "ERR_BAD_REQUEST", http.StatusBadRequest),
			Entry("checksum mismatch", errtypes.ChecksumMismatch("sha1"), "ERR_CHECKSUM_MISMATCH", errtypes.StatusChecksumMismatch),
			Entry("permission denied", errtypes.PermissionDenied("share was revoked"), "ERR_PERMISSION_DENIED", http.StatusForbidden),
		)

		It("passes an unmapped error through for tusd to answer with a 500", func() {
			session := stagedSession(ctx, store, true)
			fs.prepareErr = errors.New("blobstore unavailable")

			err := adapterFor(session).FinishUpload(ctx)

			Expect(err).To(MatchError("blobstore unavailable"))
			var tusErr tusd.Error
			Expect(errors.As(err, &tusErr)).To(BeFalse())
		})
	})

	Describe("Terminate", func() {
		// The common case: a cancel mid-transfer, before the node was created.
		It("rolls back before unmarking, and reverts nothing for an untouched node", func() {
			session := stagedSession(ctx, store, false)

			Expect(adapterFor(session).Terminate(ctx)).To(Succeed())
			Expect(fs.calls).To(Equal([]string{
				"RollbackUpload(nodeExisted=false,sizeDiff=0)",
				"MarkProcessing(false)",
			}))
		})

		// A size already propagated has to be reverted, or the quota stays consumed.
		It("reverts the size the session recorded", func() {
			session := stagedSession(ctx, store, true)
			session.SetSizeDiff(bodyLen)
			Expect(session.Persist(ctx)).To(Succeed())

			Expect(adapterFor(session).Terminate(ctx)).To(Succeed())
			Expect(fs.calls[0]).To(Equal("RollbackUpload(nodeExisted=true,sizeDiff=17)"))
		})

		It("removes the staged files", func() {
			session := stagedSession(ctx, store, false)

			Expect(adapterFor(session).Terminate(ctx)).To(Succeed())

			_, err := os.Stat(session.BinPath())
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			_, err = store.Get(ctx, session.ID())
			Expect(err).To(HaveOccurred())
		})

		// Rollback failure keeps the session so quota can be reclaimed on retry.
		It("returns an error when the driver rollback fails", func() {
			session := stagedSession(ctx, store, false)
			fs.rollbackErr = errors.New("no such node")

			Expect(adapterFor(session).Terminate(ctx)).To(MatchError(ContainSubstring("no such node")))

			_, err := os.Stat(session.BinPath())
			Expect(err).ToNot(HaveOccurred(), "session bin should be kept for retry")
		})
	})

	Describe("DeclareLength", func() {
		// A third-party-copy upload arrives without an Upload-Length.
		It("records the size and clears the deferred flag", func() {
			session := store.New(ctx)
			session.SetSizeIsDeferred(true)
			Expect(session.TouchBin()).To(Succeed())
			Expect(session.Persist(ctx)).To(Succeed())

			Expect(adapterFor(session).DeclareLength(ctx, bodyLen)).To(Succeed())

			reloaded, err := store.Get(ctx, session.ID())
			Expect(err).ToNot(HaveOccurred())
			Expect(reloaded.Size()).To(Equal(bodyLen))
			info, err := reloaded.GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.SizeIsDeferred).To(BeFalse())
		})
	})

	Describe("ConcatUploads", func() {
		It("appends the partials in the order given", func() {
			target := store.New(ctx)
			Expect(target.TouchBin()).To(Succeed())

			partials := []tusd.Upload{}
			for _, part := range []string{"hello ", "coordinator"} {
				partial := store.New(ctx)
				Expect(partial.TouchBin()).To(Succeed())
				_, err := partial.WriteChunk(ctx, 0, strings.NewReader(part))
				Expect(err).ToNot(HaveOccurred())
				partials = append(partials, adapterFor(partial))
			}

			Expect(adapterFor(target).ConcatUploads(ctx, partials)).To(Succeed())

			staged, err := os.ReadFile(target.BinPath())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(staged)).To(Equal(body))
		})

		// A retried concat must not discard the partials that already landed.
		It("keeps what the target already holds", func() {
			target := store.New(ctx)
			Expect(target.TouchBin()).To(Succeed())
			_, err := target.WriteChunk(ctx, 0, strings.NewReader("hello "))
			Expect(err).ToNot(HaveOccurred())

			partial := store.New(ctx)
			Expect(partial.TouchBin()).To(Succeed())
			_, err = partial.WriteChunk(ctx, 0, strings.NewReader("coordinator"))
			Expect(err).ToNot(HaveOccurred())

			Expect(adapterFor(target).ConcatUploads(ctx, []tusd.Upload{adapterFor(partial)})).To(Succeed())

			staged, rErr := os.ReadFile(target.BinPath())
			Expect(rErr).ToNot(HaveOccurred())
			Expect(string(staged)).To(Equal(body))
		})

		It("rejects a partial that is not one of our sessions", func() {
			target := store.New(ctx)
			Expect(target.TouchBin()).To(Succeed())

			err := adapterFor(target).ConcatUploads(ctx, []tusd.Upload{foreignUpload{}})

			Expect(err).To(MatchError(ContainSubstring("unexpected partial upload type")))
		})

		It("reports a target it cannot append to", func() {
			target := store.New(ctx)

			err := adapterFor(target).ConcatUploads(ctx, nil)

			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})

		It("reports a partial whose bytes are gone", func() {
			target := store.New(ctx)
			Expect(target.TouchBin()).To(Succeed())
			partial := store.New(ctx)

			err := adapterFor(target).ConcatUploads(ctx, []tusd.Upload{adapterFor(partial)})

			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})

		It("reports a partial it cannot read through", func() {
			target := store.New(ctx)
			Expect(target.TouchBin()).To(Succeed())
			partial := store.New(ctx)
			Expect(partial.TouchBin()).To(Succeed())

			err := adapterFor(target).ConcatUploads(ctx, []tusd.Upload{
				&tusAdapter{coord: c, session: &brokenSession{
					Session: partial,
					readErr: errors.New("input/output error"),
				}},
			})

			Expect(err).To(MatchError("input/output error"))
		})
	})

	Describe("the store interface", func() {
		It("registers itself for the core and the three extensions it implements", func() {
			composer := tusd.NewStoreComposer()

			c.UseIn(composer)

			Expect(composer.Core).To(Equal(tusd.DataStore(c)))
			Expect(composer.UsesTerminater).To(BeTrue())
			Expect(composer.UsesConcater).To(BeTrue())
			Expect(composer.UsesLengthDeferrer).To(BeTrue())
		})

		// A bare tus POST resolves no target and checks no quota.
		It("refuses to create an upload tusd was asked for directly", func() {
			_, err := c.NewUpload(ctx, tusd.FileInfo{})

			Expect(err).To(MatchError(errNotImplemented))
		})

		It("wraps the session with the requested id", func() {
			session := stagedSession(ctx, store, false)

			up, err := c.GetUpload(ctx, session.ID())

			Expect(err).ToNot(HaveOccurred())
			Expect(up.(*tusAdapter).session.ID()).To(Equal(session.ID()))
			Expect(up.(*tusAdapter).coord).To(BeIdenticalTo(c))
		})

		It("reports an unknown id as not found, which tusd answers with a 404", func() {
			_, err := c.GetUpload(ctx, "no-such-session")

			Expect(err).To(MatchError(tusd.ErrNotFound))
		})

		It("hands tusd the same adapter for each extension", func() {
			up := adapterFor(stagedSession(ctx, store, false))

			Expect(c.AsTerminatableUpload(up)).To(BeIdenticalTo(up))
			Expect(c.AsLengthDeclarableUpload(up)).To(BeIdenticalTo(up))
			Expect(c.AsConcatableUpload(up)).To(BeIdenticalTo(up))
		})
	})
})

// foreignUpload is a tusd.Upload from some other data store.
type foreignUpload struct{}

func (foreignUpload) GetInfo(context.Context) (tusd.FileInfo, error)              { return tusd.FileInfo{}, nil }
func (foreignUpload) GetReader(context.Context) (io.ReadCloser, error)            { return nil, nil }
func (foreignUpload) WriteChunk(context.Context, int64, io.Reader) (int64, error) { return 0, nil }
func (foreignUpload) FinishUpload(context.Context) error                          { return nil }
