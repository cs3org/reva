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

		storageId = "storageid"
		spaceid   = "spaceid"
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
			s := c.Get(storageId, spaceid, share1.Id.OpaqueId)
			Expect(s).To(BeNil())

			c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)

			s = c.Get(storageId, spaceid, share1.Id.OpaqueId)
			Expect(s).ToNot(BeNil())
			Expect(s).To(Equal(share1))
		})

		It("sets the mtime", func() {
			c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)
			Expect(c.Providers[storageId].Spaces[spaceid].Mtime).ToNot(Equal(time.Time{}))
		})

		It("updates the mtime", func() {
			c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)
			old := c.Providers[storageId].Spaces[spaceid].Mtime
			c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)
			Expect(c.Providers[storageId].Spaces[spaceid].Mtime).ToNot(Equal(old))
		})
	})

	Context("with an existing entry", func() {
		BeforeEach(func() {
			c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)
		})

		Describe("Remove", func() {
			It("removes the entry", func() {
				s := c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).ToNot(BeNil())
				Expect(s).To(Equal(share1))

				c.Remove(storageId, spaceid, share1.Id.OpaqueId)

				s = c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).To(BeNil())
			})

			It("updates the mtime", func() {
				c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)
				old := c.Providers[storageId].Spaces[spaceid].Mtime
				c.Remove(storageId, spaceid, share1.Id.OpaqueId)
				Expect(c.Providers[storageId].Spaces[spaceid].Mtime).ToNot(Equal(old))
			})
		})

		Describe("Persist", func() {
			It("handles non-existent storages", func() {
				Expect(c.Persist(ctx, "foo", "bar")).To(Succeed())
			})
			It("handles non-existent spaces", func() {
				Expect(c.Persist(ctx, storageId, "bar")).To(Succeed())
			})

			It("persists", func() {
				Expect(c.Persist(ctx, storageId, spaceid)).To(Succeed())
			})
		})

		Describe("Sync", func() {
			BeforeEach(func() {
				Expect(c.Persist(ctx, storageId, spaceid)).To(Succeed())
				// reset in-memory cache
				c = providercache.New(storage)
			})

			It("downloads if needed", func() {
				s := c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).To(BeNil())

				c.Sync(ctx, storageId, spaceid)

				s = c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).ToNot(BeNil())
			})

			It("does not download if not needed", func() {
				s := c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).To(BeNil())

				c.Providers[storageId] = &providercache.Spaces{
					Spaces: map[string]*providercache.Shares{
						spaceid: {
							Mtime: time.Now(),
						},
					},
				}
				c.Sync(ctx, storageId, spaceid)

				s = c.Get(storageId, spaceid, share1.Id.OpaqueId)
				Expect(s).To(BeNil()) // Sync from disk didn't happen because in-memory mtime is later than on disk
			})
		})
	})
})
