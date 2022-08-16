package providercache_test

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/providercache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
)

var _ = Describe("Cache", func() {
	var (
		c       providercache.Cache
		storage metadata.Storage

		storageID = "storageid"
		spaceID   = "spaceid"
		shareID   = "storageid$spaceid!share1"
		share1    = &collaboration.Share{
			Id: &collaboration.ShareId{
				OpaqueId: "share1",
			},
		}
		ctx    context.Context
		tmpdir string
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		tmpdir, err = ioutil.TempDir("", "providercache-test")
		Expect(err).ToNot(HaveOccurred())

		err = os.MkdirAll(tmpdir, 0755)
		Expect(err).ToNot(HaveOccurred())

		storage, err = metadata.NewDiskStorage(tmpdir)
		Expect(err).ToNot(HaveOccurred())

		c = providercache.New(storage)
		Expect(c).ToNot(BeNil())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("Add", func() {
		It("adds a share", func() {
			s := c.Get(storageID, spaceID, shareID)
			Expect(s).To(BeNil())

			c.Add(ctx, storageID, spaceID, shareID, share1)

			s = c.Get(storageID, spaceID, shareID)
			Expect(s).ToNot(BeNil())
			Expect(s).To(Equal(share1))
		})

		It("sets the mtime", func() {
			c.Add(ctx, storageID, spaceID, shareID, share1)
			Expect(c.Providers[storageID].Spaces[spaceID].Mtime).ToNot(Equal(time.Time{}))
		})

		It("updates the mtime", func() {
			c.Add(ctx, storageID, spaceID, shareID, share1)
			old := c.Providers[storageID].Spaces[spaceID].Mtime
			c.Add(ctx, storageID, spaceID, shareID, share1)
			Expect(c.Providers[storageID].Spaces[spaceID].Mtime).ToNot(Equal(old))
		})
	})

	Context("with an existing entry", func() {
		BeforeEach(func() {
			Expect(c.Add(ctx, storageID, spaceID, shareID, share1)).To(Succeed())
		})

		Describe("Get", func() {
			It("returns the entry", func() {
				s := c.Get(storageID, spaceID, shareID)
				Expect(s).ToNot(BeNil())
			})
		})

		Describe("Remove", func() {
			It("removes the entry", func() {
				s := c.Get(storageID, spaceID, shareID)
				Expect(s).ToNot(BeNil())
				Expect(s).To(Equal(share1))

				c.Remove(ctx, storageID, spaceID, shareID)

				s = c.Get(storageID, spaceID, shareID)
				Expect(s).To(BeNil())
			})

			It("updates the mtime", func() {
				c.Add(ctx, storageID, spaceID, shareID, share1)
				old := c.Providers[storageID].Spaces[spaceID].Mtime
				c.Remove(ctx, storageID, spaceID, shareID)
				Expect(c.Providers[storageID].Spaces[spaceID].Mtime).ToNot(Equal(old))
			})
		})

		Describe("Persist", func() {
			It("handles non-existent storages", func() {
				Expect(c.Persist(ctx, "foo", "bar")).To(Succeed())
			})
			It("handles non-existent spaces", func() {
				Expect(c.Persist(ctx, storageID, "bar")).To(Succeed())
			})

			It("persists", func() {
				Expect(c.Persist(ctx, storageID, spaceID)).To(Succeed())
			})

			It("updates the mtime", func() {
				oldMtime := c.Providers[storageID].Spaces[spaceID].Mtime

				Expect(c.Persist(ctx, storageID, spaceID)).To(Succeed())
				Expect(c.Providers[storageID].Spaces[spaceID].Mtime).ToNot(Equal(oldMtime))
			})

			It("does not persist if the etag changed on disk", func() {
				c.Providers[storageID].Spaces[spaceID].Mtime = time.Now().Add(-3 * time.Hour)

				Expect(c.Persist(ctx, storageID, spaceID)).ToNot(Succeed())
			})
		})

		Describe("Sync", func() {
			BeforeEach(func() {
				Expect(c.Persist(ctx, storageID, spaceID)).To(Succeed())
				// reset in-memory cache
				c = providercache.New(storage)
			})

			It("downloads if needed", func() {
				s := c.Get(storageID, spaceID, shareID)
				Expect(s).To(BeNil())

				c.Sync(ctx, storageID, spaceID)

				s = c.Get(storageID, spaceID, shareID)
				Expect(s).ToNot(BeNil())
			})

			It("does not download if not needed", func() {
				s := c.Get(storageID, spaceID, shareID)
				Expect(s).To(BeNil())

				c.Providers[storageID] = &providercache.Spaces{
					Spaces: map[string]*providercache.Shares{
						spaceID: {
							Mtime: time.Now(),
						},
					},
				}
				c.Sync(ctx, storageID, spaceID) // Sync from disk won't happen because in-memory mtime is later than on disk

				s = c.Get(storageID, spaceID, shareID)
				Expect(s).To(BeNil())
			})
		})
	})
})
