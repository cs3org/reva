// Copyright 2018-2022 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package receivedsharecache_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/owncloud/reva/v2/pkg/share/manager/jsoncs3/receivedsharecache"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c       receivedsharecache.Cache
		storage metadata.Storage

		userID  = "user"
		spaceID = "spaceid"
		shareID = "storageid$spaceid!share1"
		share   = &collaboration.Share{
			Id: &collaboration.ShareId{
				OpaqueId: shareID,
			},
		}
		ctx    context.Context
		tmpdir string
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		tmpdir, err = os.MkdirTemp("", "providercache-test")
		Expect(err).ToNot(HaveOccurred())

		err = os.MkdirAll(tmpdir, 0755)
		Expect(err).ToNot(HaveOccurred())

		storage, err = metadata.NewDiskStorage(tmpdir)
		Expect(err).ToNot(HaveOccurred())

		c = receivedsharecache.New(storage, 0*time.Second)
		Expect(&c).ToNot(BeNil())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("Add", func() {
		It("adds an entry", func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaboration.ShareState_SHARE_STATE_PENDING,
			}
			err := c.Add(ctx, userID, spaceID, rs)
			Expect(err).ToNot(HaveOccurred())

			s, err := c.Get(ctx, userID, spaceID, shareID)
			Expect(err).ToNot(HaveOccurred())
			Expect(s).ToNot(BeNil())
		})

		It("persists the new entry", func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaboration.ShareState_SHARE_STATE_PENDING,
			}
			err := c.Add(ctx, userID, spaceID, rs)
			Expect(err).ToNot(HaveOccurred())

			c = receivedsharecache.New(storage, 0*time.Second)
			s, err := c.Get(ctx, userID, spaceID, shareID)
			Expect(err).ToNot(HaveOccurred())
			Expect(s).ToNot(BeNil())
		})
	})

	Describe("concurrent writes from multiple cache instances", func() {
		It("preserves all shares when replicas write concurrently for the same user", func() {
			const numReplicas = 3
			const numShares = 15

			// barrierStorage holds all Upload calls until numReplicas have arrived,
			// then releases them simultaneously. This makes the race deterministic
			// regardless of OS goroutine scheduling or GOMAXPROCS.
			bs := newBarrierStorage(storage, numReplicas)
			replicas := make([]receivedsharecache.Cache, numReplicas)
			for i := range replicas {
				replicas[i] = receivedsharecache.New(bs, 0*time.Second)
			}

			errs := make([]error, numShares)
			var wg sync.WaitGroup
			for i := 0; i < numShares; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					rs := &collaboration.ReceivedShare{
						Share: &collaboration.Share{
							Id: &collaboration.ShareId{OpaqueId: fmt.Sprintf("share-%d", idx)},
						},
						State: collaboration.ShareState_SHARE_STATE_PENDING,
					}
					errs[idx] = replicas[idx%numReplicas].Add(ctx, userID, spaceID, rs)
				}(i)
			}
			wg.Wait()
			for i, err := range errs {
				Expect(err).ToNot(HaveOccurred(), "Add failed for share-%d", i)
			}

			fresh := receivedsharecache.New(storage, 0*time.Second)
			spaces, err := fresh.List(ctx, userID)
			Expect(err).ToNot(HaveOccurred())
			Expect(spaces[spaceID]).ToNot(BeNil())
			for i := 0; i < numShares; i++ {
				Expect(spaces[spaceID].States).To(HaveKey(fmt.Sprintf("share-%d", i)))
			}
		})
	})

	Describe("with an existing entry", func() {
		BeforeEach(func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaboration.ShareState_SHARE_STATE_PENDING,
			}
			Expect(c.Add(ctx, userID, spaceID, rs)).To(Succeed())
		})

		Describe("Get", func() {
			It("handles unknown users", func() {
				s, err := c.Get(ctx, "something", spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(BeNil())
			})

			It("handles unknown spaces", func() {
				s, err := c.Get(ctx, userID, "something", shareID)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(BeNil())
			})

			It("handles unknown shares", func() {
				s, err := c.Get(ctx, userID, spaceID, "something")
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(BeNil())
			})

			It("gets the entry", func() {
				s, err := c.Get(ctx, userID, spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).ToNot(BeNil())
			})
		})

		Describe("Remove", func() {
			It("removes the entry", func() {
				err := c.Remove(ctx, userID, spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())

				s, err := c.Get(ctx, userID, spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(BeNil())
			})

			It("persists the removal", func() {
				err := c.Remove(ctx, userID, spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())

				c = receivedsharecache.New(storage, 0*time.Second)
				s, err := c.Get(ctx, userID, spaceID, shareID)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(BeNil())
			})
		})
	})
})

// barrierStorage wraps a Storage and holds Upload calls until n goroutines have
// arrived, then releases them all at once. This makes the concurrent-write race
// reproducible regardless of OS goroutine scheduling.
type barrierStorage struct {
	metadata.Storage
	arrived   int32
	n         int32
	ready     chan struct{}
	closeOnce sync.Once
}

func newBarrierStorage(s metadata.Storage, n int) *barrierStorage {
	return &barrierStorage{Storage: s, n: int32(n), ready: make(chan struct{})}
}

func (b *barrierStorage) Upload(ctx context.Context, req metadata.UploadRequest) (*metadata.UploadResponse, error) {
	if atomic.AddInt32(&b.arrived, 1) >= b.n {
		b.closeOnce.Do(func() { close(b.ready) })
	}
	<-b.ready
	return b.Storage.Upload(ctx, req)
}
