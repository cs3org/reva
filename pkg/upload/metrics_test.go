package upload

import (
	"context"
	"errors"
	"path/filepath"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/rhttp/datatx/metrics"
	"github.com/owncloud/reva/v2/pkg/storage"
)

// Every upload that increments the gauge has to decrement it again.
var _ = Describe("the processing gauge", func() {
	var (
		ctx   context.Context
		store *FileStore
		fs    *fakeFS
		c     *coordinator
	)

	// delta is how far the gauge moved, the metric being shared by the whole suite.
	delta := func(body func()) float64 {
		before := testutil.ToFloat64(metrics.UploadProcessing)
		body()
		return testutil.ToFloat64(metrics.UploadProcessing) - before
	}

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id:       &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
			Username: "alice",
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

	It("returns to where it was after a committed upload", func() {
		Expect(delta(func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).ToNot(HaveOccurred())
		})).To(BeZero())
	})

	It("returns to where it was after a rejected prepare", func() {
		fs.prepareErr = errors.New("precondition failed")

		Expect(delta(func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).To(HaveOccurred())
		})).To(BeZero())
	})

	It("returns to where it was after a rejected commit", func() {
		fs.commitErr = errors.New("blobstore unavailable")

		Expect(delta(func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).To(HaveOccurred())
		})).To(BeZero())
	})

	It("returns to where it was after a mismatching checksum", func() {
		session := stagedSession(ctx, store, false)
		session.SetMetadata("checksum", "sha1 0000000000000000000000000000000000000000")
		Expect(session.Persist(ctx)).To(Succeed())

		Expect(delta(func() {
			_, err := c.finishUpload(ctx, session)
			Expect(err).To(HaveOccurred())
		})).To(BeZero())
	})

	// The mark never went through, so there is nothing to take back off the gauge.
	It("does not move when the node cannot be flagged", func() {
		fs.markErr = errors.New("flock timeout")

		Expect(delta(func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).To(HaveOccurred())
		})).To(BeZero())
	})

	// A cancel arrives before the mark, so before the gauge ever moved.
	It("does not move when a transfer is cancelled", func() {
		session := stagedSession(ctx, store, false)

		Expect(delta(func() {
			Expect((&tusAdapter{session: session, coord: c}).Terminate(ctx)).To(Succeed())
		})).To(BeZero())
	})

	// Postprocessing owns the node until it reports back.
	It("stays up for an upload handed to postprocessing", func() {
		pub := &fakePublisher{}
		c = NewCoordinator(fs, store, "", pub)
		c.async = true
		fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}

		Expect(delta(func() {
			_, err := c.finishUpload(ctx, stagedSession(ctx, store, false))
			Expect(err).ToNot(HaveOccurred())
		})).To(Equal(float64(1)))
	})
})
