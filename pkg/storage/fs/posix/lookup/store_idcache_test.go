package lookup_test

import (
	"context"

	"github.com/opencloud-eu/reva/v2/pkg/storage/cache"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StoreIdcache", func() {
	var (
		storeIdcache *lookup.StoreIDCache
	)

	BeforeEach(func() {
		storeIdcache = lookup.NewStoreIDCache(cache.Config{
			Store:    "memory",
			Database: "idcache",
			Size:     100,
			Nodes:    []string{"localhost:2379"},
		})

		Expect(storeIdcache.Set(context.TODO(), "spaceID", "nodeID", "path")).To(Succeed())
	})

	Describe("StoreIdcache", func() {
		Describe("NewStoreIDCache", func() {
			It("should return a new StoreIDCache", func() {
				Expect(storeIdcache).ToNot(BeNil())
			})
		})

		Describe("Delete", func() {
			It("should delete an entry from the cache", func() {
				v, ok := storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("path"))

				err := storeIdcache.Delete(context.TODO(), "spaceID", "nodeID")
				Expect(err).ToNot(HaveOccurred())

				_, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeFalse())
			})
		})

		Describe("DeleteByPath", func() {
			It("should delete an entry from the cache", func() {
				v, ok := storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("path"))

				err := storeIdcache.DeleteByPath(context.TODO(), "path")
				Expect(err).ToNot(HaveOccurred())

				_, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeFalse())
			})

			It("should not delete an entry from the cache if the path does not exist", func() {
				err := storeIdcache.DeleteByPath(context.TODO(), "nonexistent")
				Expect(err).ToNot(HaveOccurred())
			})

			It("deletes recursively", func() {
				Expect(storeIdcache.Set(context.TODO(), "spaceID", "nodeID", "path")).To(Succeed())
				Expect(storeIdcache.Set(context.TODO(), "spaceID", "nodeID2", "path/child")).To(Succeed())
				Expect(storeIdcache.Set(context.TODO(), "spaceID", "nodeID3", "path/child2")).To(Succeed())

				v, ok := storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("path"))
				v, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID2")
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("path/child"))
				v, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID3")
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("path/child2"))

				err := storeIdcache.DeleteByPath(context.TODO(), "path")
				Expect(err).ToNot(HaveOccurred())

				_, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID")
				Expect(ok).To(BeFalse())
				_, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID2")
				Expect(ok).To(BeFalse())
				_, ok = storeIdcache.Get(context.TODO(), "spaceID", "nodeID3")
				Expect(ok).To(BeFalse())
			})
		})
	})
})
