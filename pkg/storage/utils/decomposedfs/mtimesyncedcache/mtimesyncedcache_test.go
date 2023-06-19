package mtimesyncedcache_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/mtimesyncedcache"
)

var _ = Describe("Mtimesyncedcache", func() {
	var (
		cache mtimesyncedcache.Cache[string, string]

		key   = "key"
		value = "value"
	)

	BeforeEach(func() {
		cache = mtimesyncedcache.New[string, string]()
	})

	Describe("Store", func() {
		It("stores a value", func() {
			time := time.Now()

			err := cache.Store(key, time, value)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Load", func() {
		It("loads the stored value", func() {
			err := cache.Store(key, time.Now(), value)
			Expect(err).ToNot(HaveOccurred())

			v, ok := cache.Load(key)
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(value))
		})

		It("reports when the key doesn't exist", func() {
			_, ok := cache.Load("doesnotexist")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("LoadOrStore", func() {
		It("does not update the cache if the cache is up to date", func() {
			cachedTime := time.Now().Add(-1 * time.Hour)
			err := cache.Store(key, cachedTime, value)
			Expect(err).ToNot(HaveOccurred())

			newvalue := "yaaay"
			v, err := cache.LoadOrStore(key, cachedTime, func() (string, error) {
				return newvalue, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(value))

			v, err = cache.LoadOrStore(key, time.Now().Add(-2*time.Hour), func() (string, error) {
				return newvalue, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(value))
		})

		It("updates the cache if the cache is outdated", func() {
			outdatedTime := time.Now().Add(-1 * time.Hour)
			err := cache.Store(key, outdatedTime, value)
			Expect(err).ToNot(HaveOccurred())

			newvalue := "yaaay"
			v, err := cache.LoadOrStore(key, time.Now(), func() (string, error) {
				return newvalue, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(newvalue))
		})

		It("stores the value if the key doesn't exist yet", func() {
			newvalue := "yaaay"
			v, err := cache.LoadOrStore(key, time.Now(), func() (string, error) {
				return newvalue, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(newvalue))
		})

		It("sets the mtime when storing the value", func() {
			newTime := time.Now()

			newvalue := "yaaay"
			v, err := cache.LoadOrStore(key, newTime, func() (string, error) {
				return newvalue, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(newvalue))

			newvalue2 := "asdfasdf"
			v, err = cache.LoadOrStore(key, newTime, func() (string, error) {
				return newvalue2, nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(newvalue))
		})

		It("passes on error from the store func", func() {
			v, err := cache.LoadOrStore(key, time.Now(), func() (string, error) {
				return "", errors.New("baa")
			})
			Expect(v).To(Equal(""))
			Expect(err.Error()).To(Equal("baa"))

		})
	})
})
