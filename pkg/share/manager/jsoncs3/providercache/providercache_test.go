package providercache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/providercache"
	storagemocks "github.com/cs3org/reva/v2/pkg/storage/utils/metadata/mocks"
)

var _ = Describe("Cache", func() {
	var (
		c       providercache.Cache
		storage *storagemocks.Storage

		// storageId = "storageid"
		// spaceid   = "spaceid"
		// share1    = &collaboration.Share{
		// 	Id: &collaboration.ShareId{
		// 		OpaqueId: "share1",
		// 	},
		// }
	)

	BeforeEach(func() {
		storage = &storagemocks.Storage{}

		c = providercache.New(storage)
		Expect(c).ToNot(BeNil())
	})

	Describe("Add", func() {
		It("adds a share", func() {
			// oldMtime := c.Providers.
			// s := c.Get(storageId, spaceid, share1.Id.OpaqueId)
			// Expect(s).To(BeNil())

			// c.Add(storageId, spaceid, share1.Id.OpaqueId, share1)

			// s = c.Get(storageId, spaceid, share1.Id.OpaqueId)
			// Expect(s).ToNot(BeNil())
			// Expect(s).To(Equal(share1))
		})
	})
})
