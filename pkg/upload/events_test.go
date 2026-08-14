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
	"github.com/owncloud/reva/v2/pkg/events"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/utils"
)

var _ = Describe("coordinator events", func() {
	var (
		ctx   context.Context
		store *FileStore
		fs    *fakeFS
		pub   *fakePublisher
		c     *coordinator
	)

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id:       &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
			Username: "alice",
		})
		store = NewFileStore(filepath.Join(GinkgoT().TempDir(), "uploads"), TokenOptions{
			DataGatewayEndpoint:  "https://cloud.example.com/data",
			DownloadEndpoint:     "https://cloud.example.com/data/",
			TransferSharedSecret: "secret",
			TransferExpires:      3600,
		}, nopLog())
		Expect(store.Setup()).To(Succeed())

		fs = &fakeFS{
			touched: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			},
		}
		pub = &fakePublisher{}
		c = NewCoordinator(fs, store, "", pub)
	})

	// Consumers such as the search indexer only listen for UploadReady.
	Describe("UploadReady", func() {
		It("announces an inline commit", func() {
			session := stagedSession(ctx, store, false)

			ri, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())

			Expect(pub.published).To(HaveLen(1))
			ready := pub.published[0].(events.UploadReady)
			Expect(ready.UploadID).To(Equal(session.ID()))
			Expect(ready.Filename).To(Equal("report.docx"))
			Expect(ready.ResourceID).To(Equal(ri.GetId()))
			Expect(ready.ExecutingUser.GetUsername()).To(Equal("alice"))
			Expect(ready.IsVersion).To(BeFalse())
		})

		// The indexer resolves against the space root plus the path within it.
		It("carries a reference rooted at the space", func() {
			session := stagedSession(ctx, store, false)

			_, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())

			ref := pub.published[0].(events.UploadReady).FileRef
			Expect(ref.GetResourceId().GetStorageId()).To(Equal(mountID))
			Expect(ref.GetResourceId().GetSpaceId()).To(Equal(spaceRoot))
			Expect(ref.GetResourceId().GetOpaqueId()).To(Equal(spaceRoot))
			Expect(ref.GetPath()).To(Equal("./Shares/project/report.docx"))
		})

		It("reports an overwrite that snapshotted a version", func() {
			session := stagedSession(ctx, store, true)
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen, VersionCreated: true}

			_, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())

			Expect(pub.published[0].(events.UploadReady).IsVersion).To(BeTrue())
		})

		// The upload is already committed by then.
		It("does not fail the upload when it cannot be published", func() {
			pub.err = errors.New("nats unreachable")

			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))

			Expect(err).ToNot(HaveOccurred())
		})

		It("is skipped when no publisher is wired", func() {
			c = NewCoordinator(fs, store, "", nil)

			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))

			Expect(err).ToNot(HaveOccurred())
			Expect(pub.published).To(BeEmpty())
		})

		// Public link and OCM auth borrow an identity, the real actor riding in the opaque.
		It("names the real actor behind a borrowed identity", func() {
			impersonated := &userpb.User{
				Id:       &userpb.UserId{OpaqueId: "link-share", Idp: "idp.example.com"},
				Username: "link-share",
			}
			impersonated.Opaque = utils.AppendJSONToOpaque(nil, "impersonating-user", &userpb.User{
				Id:       &userpb.UserId{OpaqueId: "bob", Idp: "idp.example.com"},
				Username: "bob",
			})

			_, err := c.finishUpload(ctxpkg.ContextSetUser(ctx, impersonated), stagedSession(ctx, store, false))
			Expect(err).ToNot(HaveOccurred())

			Expect(pub.published[0].(events.UploadReady).ImpersonatingUser.GetUsername()).To(Equal("bob"))
		})

		It("names nobody when the opaque holds no impersonating user", func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).ToNot(HaveOccurred())

			Expect(pub.published[0].(events.UploadReady).ImpersonatingUser).To(BeNil())
		})

		It("names nobody when the opaque cannot be read", func() {
			broken := &userpb.User{Id: &userpb.UserId{OpaqueId: "link-share"}}
			broken.Opaque = utils.AppendPlainToOpaque(nil, "impersonating-user", "not json")

			_, err := c.finishUpload(ctxpkg.ContextSetUser(ctx, broken), stagedSession(ctx, store, false))
			Expect(err).ToNot(HaveOccurred())

			Expect(pub.published[0].(events.UploadReady).ImpersonatingUser).To(BeNil())
		})
	})

	// Postprocessing scans the staged bytes before the commit is allowed to run.
	Describe("BytesReceived", func() {
		BeforeEach(func() {
			c.async = true
		})

		It("hands the staged upload to postprocessing instead of committing", func() {
			session := stagedSession(ctx, store, false)

			ri, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(ri.GetEtag()).To(Equal("etag-after-commit"))
			Expect(fs.calls).ToNot(ContainElement(ContainSubstring("CommitUpload")))
			Expect(pub.published).To(HaveLen(1))
			received := pub.published[0].(events.BytesReceived)
			Expect(received.UploadID).To(Equal(session.ID()))
			Expect(received.Filesize).To(Equal(uint64(bodyLen)))
			Expect(received.ResourceID.GetOpaqueId()).To(Equal(nodeID))
			Expect(received.URL).To(HavePrefix("https://cloud.example.com/data/"))
		})

		// Postprocessing needs both to finish later.
		It("keeps the node flagged and the bytes staged", func() {
			session := stagedSession(ctx, store, false)

			_, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())

			Expect(fs.calls).ToNot(ContainElement("MarkProcessing(false)"))
			_, sErr := os.Stat(session.BinPath())
			Expect(sErr).ToNot(HaveOccurred())
			_, gErr := store.Get(ctx, session.ID())
			Expect(gErr).ToNot(HaveOccurred())
		})

		// Nobody will pick the upload up, so leaving it flagged strands the file.
		It("rolls the prepared upload back when the event cannot be published", func() {
			session := stagedSession(ctx, store, true)
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}
			pub.err = errors.New("nats unreachable")

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("nats unreachable"))
			Expect(fs.calls).To(ContainElement("RollbackUpload(nodeExisted=true,sizeDiff=17)"))
			Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
		})

		// Postprocessing has no way to reach the bytes without that URL.
		It("rolls back when the download URL cannot be signed", func() {
			session := &brokenSession{
				Session: stagedSession(ctx, store, true),
				urlErr:  errors.New("could not sign transfer token"),
			}
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}

			_, err := c.finishUpload(ctx, session)

			Expect(err).To(MatchError("could not sign transfer token"))
			Expect(fs.calls).To(ContainElement("RollbackUpload(nodeExisted=true,sizeDiff=17)"))
			Expect(pub.published).To(BeEmpty())
		})

		// A zero-length upload has no bytes to scan.
		It("commits an empty upload inline", func() {
			session := stagedSession(ctx, store, false)
			session.SetSize(0)
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(ContainElement("CommitUpload(length=0)"))
		})

		// Async on with no publisher wired has to fall back rather than panic.
		It("commits inline when no publisher is wired", func() {
			c = NewCoordinator(fs, store, "", nil)
			c.async = true

			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).To(ContainElement("CommitUpload(length=17)"))
		})
	})

	// A committed blob always carries the verdict it was cleared under.
	Describe("the scan verdict", func() {
		It("passes the recorded verdict to the commit", func() {
			session := stagedSession(ctx, store, false)
			session.SetScanData("clean", time.Unix(1700000000, 0))
			Expect(session.Persist(ctx)).To(Succeed())

			_, err := c.finishUpload(ctx, session)

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.committed.ScanResult).To(Equal("clean"))
			Expect(fs.committed.ScanDate).To(BeTemporally("==", time.Unix(1700000000, 0)))
		})

		It("leaves it empty on the inline path, which never scans", func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))

			Expect(err).ToNot(HaveOccurred())
			Expect(fs.committed.ScanResult).To(BeEmpty())
		})
	})

	Describe("ListUploadSessions", func() {
		// Only the driver can resolve a session's node.
		It("refuses the orphaned filter", func() {
			orphaned := true

			_, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{Orphaned: &orphaned})

			Expect(err).To(BeAssignableToTypeOf(errtypes.NotSupported("")))
		})

		It("returns every session when no filter is given", func() {
			first := stagedSession(ctx, store, false)
			second := stagedSession(ctx, store, false)

			sessions, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{})

			Expect(err).ToNot(HaveOccurred())
			Expect(sessions).To(HaveLen(2))
			Expect([]string{sessions[0].ID(), sessions[1].ID()}).To(ConsistOf(first.ID(), second.ID()))
		})

		It("looks a single session up by id", func() {
			wanted := stagedSession(ctx, store, false)
			stagedSession(ctx, store, false)
			id := wanted.ID()

			sessions, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{ID: &id})

			Expect(err).ToNot(HaveOccurred())
			Expect(sessions).To(HaveLen(1))
			Expect(sessions[0].ID()).To(Equal(id))
		})

		It("reports an unknown id as an error", func() {
			id := "no-such-session"

			_, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{ID: &id})

			Expect(err).To(HaveOccurred())
		})

		It("propagates a failure to read the sessions", func() {
			c = NewCoordinator(fs, &brokenStore{
				SessionStore: store,
				listErr:      errors.New("upload directory unreadable"),
			}, "", nil)

			_, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{})

			Expect(err).To(MatchError("upload directory unreadable"))
		})

		It("filters on whether all the bytes have arrived", func() {
			complete := stagedSession(ctx, store, false)
			partial := stagedSession(ctx, store, false)
			partial.SetSize(bodyLen * 2)
			Expect(partial.Persist(ctx)).To(Succeed())
			processing := true

			sessions, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{Processing: &processing})

			Expect(err).ToNot(HaveOccurred())
			Expect(sessions).To(HaveLen(1))
			Expect(sessions[0].ID()).To(Equal(complete.ID()))
		})

		It("filters on expiry", func() {
			expired := stagedSession(ctx, store, false)
			expired.SetMetadata("expires", utils.TimeToOCMtime(time.Now().Add(-time.Hour)))
			Expect(expired.Persist(ctx)).To(Succeed())
			fresh := stagedSession(ctx, store, false)
			fresh.SetMetadata("expires", utils.TimeToOCMtime(time.Now().Add(time.Hour)))
			Expect(fresh.Persist(ctx)).To(Succeed())

			isExpired := true
			gone, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{Expired: &isExpired})
			Expect(err).ToNot(HaveOccurred())
			Expect(gone).To(HaveLen(1))
			Expect(gone[0].ID()).To(Equal(expired.ID()))

			isExpired = false
			live, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{Expired: &isExpired})
			Expect(err).ToNot(HaveOccurred())
			Expect(live).To(HaveLen(1))
			Expect(live[0].ID()).To(Equal(fresh.ID()))
		})

		// A non-empty scan result is a virus report; a clean scan records nothing.
		It("filters on the scan verdict", func() {
			infected := stagedSession(ctx, store, false)
			infected.SetScanData("Eicar-Test-Signature", time.Now())
			Expect(infected.Persist(ctx)).To(Succeed())
			clean := stagedSession(ctx, store, false)

			hasVirus := true
			found, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{HasVirus: &hasVirus})
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(HaveLen(1))
			Expect(found[0].ID()).To(Equal(infected.ID()))

			hasVirus = false
			rest, err := c.ListUploadSessions(ctx, storage.UploadSessionFilter{HasVirus: &hasVirus})
			Expect(err).ToNot(HaveOccurred())
			Expect(rest).To(HaveLen(1))
			Expect(rest[0].ID()).To(Equal(clean.ID()))
		})
	})
})
