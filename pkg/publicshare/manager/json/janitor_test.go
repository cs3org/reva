package json_test

import (
	"context"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/publicshare"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakePersistence is a minimal, controllable persistence.Persistence used to
// observe and delay the janitor's Read/Write calls without needing a real
// storage backend.
type fakePersistence struct {
	mu sync.Mutex

	data       persistence.PublicShares
	readCount  int
	writeCount int

	// readStarted, if non-nil, receives once per Read call (non-blocking).
	readStarted chan struct{}
	// readBlockFor makes Read pretend to do work for this long.
	readBlockFor time.Duration
	// respectCtx makes the simulated work abort early on ctx.Done(), like a
	// context-aware network call would; false simulates a blocking call that
	// cannot be interrupted, like a plain disk read.
	respectCtx bool
}

func (p *fakePersistence) Init(context.Context) error { return nil }

func (p *fakePersistence) Read(ctx context.Context) (persistence.PublicShares, error) {
	p.mu.Lock()
	p.readCount++
	block := p.readBlockFor
	respectCtx := p.respectCtx
	p.mu.Unlock()

	if p.readStarted != nil {
		select {
		case p.readStarted <- struct{}{}:
		default:
		}
	}

	if block > 0 {
		if respectCtx {
			select {
			case <-time.After(block):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			time.Sleep(block)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return persistence.Copy(p.data), nil
}

func (p *fakePersistence) Write(_ context.Context, db persistence.PublicShares) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeCount++
	p.data = persistence.Copy(db)
	return nil
}

func (p *fakePersistence) snapshot() (shares int, reads int, writes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.data), p.readCount, p.writeCount
}

var _ = Describe("Janitor", func() {
	var (
		user1 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "admin",
			},
		}
		sharedResource = &providerv1beta1.ResourceInfo{
			Id: &providerv1beta1.ResourceId{
				StorageId: "storageid",
				OpaqueId:  "opaqueid",
			},
			ArbitraryMetadata: &providerv1beta1.ArbitraryMetadata{
				Metadata: map[string]string{"name": "publicshare"},
			},
		}

		ctx = ctxpkg.ContextSetUser(context.Background(), user1)
	)

	Describe("Close", func() {
		It("cancels an in-flight cleanup that honors context and returns promptly", func() {
			fp := &fakePersistence{
				data:         persistence.PublicShares{},
				readStarted:  make(chan struct{}, 1),
				readBlockFor: 2 * time.Second,
				respectCtx:   true,
			}
			m, err := json.New("https://localhost:9200", 11, 1, true, fp)
			Expect(err).ToNot(HaveOccurred())

			Eventually(fp.readStarted, 3*time.Second).Should(Receive())

			start := time.Now()
			err = m.(publicshare.ClosableManager).Close(context.Background())
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically("<", 1*time.Second),
				"Close should cancel the in-flight Read instead of waiting out its 2s block")
		})

		It("blocks until an in-flight cleanup that ignores context actually returns", func() {
			fp := &fakePersistence{
				data:         persistence.PublicShares{},
				readStarted:  make(chan struct{}, 1),
				readBlockFor: 500 * time.Millisecond,
				respectCtx:   false,
			}
			m, err := json.New("https://localhost:9200", 11, 1, true, fp)
			Expect(err).ToNot(HaveOccurred())

			Eventually(fp.readStarted, 3*time.Second).Should(Receive())

			start := time.Now()
			err = m.(publicshare.ClosableManager).Close(context.Background())
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 400*time.Millisecond),
				"Close should wait for the running cleanup to actually return, not just for cancellation")
		})

		It("returns the caller's context error if the janitor doesn't stop in time", func() {
			fp := &fakePersistence{
				data:         persistence.PublicShares{},
				readStarted:  make(chan struct{}, 1),
				readBlockFor: 300 * time.Millisecond,
				respectCtx:   false,
			}
			m, err := json.New("https://localhost:9200", 11, 1, true, fp)
			Expect(err).ToNot(HaveOccurred())

			Eventually(fp.readStarted, 3*time.Second).Should(Receive())

			closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()

			err = m.(publicshare.ClosableManager).Close(closeCtx)
			Expect(err).To(MatchError(context.DeadlineExceeded))
		})

		It("returns immediately when cleanup is disabled", func() {
			fp := &fakePersistence{data: persistence.PublicShares{}}
			m, err := json.New("https://localhost:9200", 11, 60, false, fp)
			Expect(err).ToNot(HaveOccurred())

			err = m.(publicshare.ClosableManager).Close(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("cleanupExpiredShares", func() {
		It("revokes every expired share in a single Read/Write pass", func() {
			fp := &fakePersistence{data: persistence.PublicShares{}}
			m, err := json.New("https://localhost:9200", 11, 1, true, fp)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				_ = m.(publicshare.ClosableManager).Close(context.Background())
			}()

			past := &typespb.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
			future := &typespb.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}

			for range 3 {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{Expiration: past})
				Expect(err).ToNot(HaveOccurred())
			}
			_, err = m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{Expiration: future})
			Expect(err).ToNot(HaveOccurred())

			_, _, writesBefore := fp.snapshot()

			Eventually(func() int {
				shares, _, _ := fp.snapshot()
				return shares
			}, 5*time.Second, 50*time.Millisecond).Should(Equal(1), "only the non-expired share should remain")

			_, _, writesAfter := fp.snapshot()
			Expect(writesAfter-writesBefore).To(Equal(1),
				"revoking 3 expired shares should take exactly one Write, not one per share")
		})

		It("does not write anything when there is nothing to revoke", func() {
			fp := &fakePersistence{data: persistence.PublicShares{}}
			m, err := json.New("https://localhost:9200", 11, 1, true, fp)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				_ = m.(publicshare.ClosableManager).Close(context.Background())
			}()

			future := &typespb.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}
			_, err = m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{Expiration: future})
			Expect(err).ToNot(HaveOccurred())

			_, _, writesBefore := fp.snapshot()

			// give a couple of janitor ticks a chance to run
			Consistently(func() int {
				_, _, writes := fp.snapshot()
				return writes
			}, 2500*time.Millisecond, 100*time.Millisecond).Should(Equal(writesBefore))
		})
	})
})
