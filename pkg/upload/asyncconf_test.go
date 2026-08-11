package upload

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AsyncConfFromDriverConf", func() {
	It("is disabled unless the driver asks for async uploads", func() {
		Expect(AsyncConfFromDriverConf(nil).Enabled).To(BeFalse())
		Expect(AsyncConfFromDriverConf(map[string]interface{}{"root": "/x"}).Enabled).To(BeFalse())
		Expect(AsyncConfFromDriverConf(map[string]interface{}{"asyncfileuploads": false}).Enabled).To(BeFalse())
		Expect(AsyncConfFromDriverConf(map[string]interface{}{"asyncfileuploads": true}).Enabled).To(BeTrue())
	})

	// The coordinator takes over the driver's subscription, so it has to resolve the
	// group to the same value the driver does, default included.
	It("defaults the consumer group to the driver's default", func() {
		Expect(AsyncConfFromDriverConf(map[string]interface{}{}).ConsumerGroup).To(Equal("dcfs"))
		Expect(AsyncConfFromDriverConf(map[string]interface{}{
			"events": map[string]interface{}{"numconsumers": 3},
		}).ConsumerGroup).To(Equal("dcfs"))
	})

	It("reads an explicit subscription", func() {
		ac := AsyncConfFromDriverConf(map[string]interface{}{
			"asyncfileuploads": true,
			"mount_id":         "storage-users-1",
			"events": map[string]interface{}{
				"consumer_group": "custom",
				"numconsumers":   4,
			},
		})

		Expect(ac.ConsumerGroup).To(Equal("custom"))
		Expect(ac.NumConsumers).To(Equal(4))
		Expect(ac.MountID).To(Equal("storage-users-1"))
	})
})
