package lookup_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/test-go/testify/mock"

	mocks "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/lookup/mocks"
	helpers "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/testhelpers"
)

var _ = Describe("Lookup", func() {
	var (
		env *helpers.TestEnv
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(map[string]any{})
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("GenerateSpaceId", func() {
		Context("with project spaces", func() {
			It("should generate a project space id", func() {
				spaceId, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId).ToNot(BeEmpty())
			})

			It("should not reuse exising ids", func() {
				spaceId, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId).ToNot(BeEmpty())

				spaceId2, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId2).ToNot(BeEmpty())
				Expect(spaceId).ToNot(Equal(spaceId2))
			})
		})

		Context("with personal spaces", func() {
			It("should generate a personal space id", func() {
				spaceId, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId).ToNot(BeEmpty())
			})

			It("should reuse exising ids", func() {
				spaceId, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId).ToNot(BeEmpty())

				spaceId2, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId2).ToNot(BeEmpty())
				Expect(spaceId).To(Equal(spaceId2))
			})

			It("should not generate a new id but pick up the one from disk instead when reading from the cache fails", func() {
				spaceId, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId).ToNot(BeEmpty())

				mockCache := mocks.NewIDCache(GinkgoT())
				env.Lookup.IDCache = mockCache
				mockCache.EXPECT().GetByPath(mock.Anything, mock.Anything).Return("", "", false)

				spaceId2, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceId2).ToNot(BeEmpty())
				Expect(spaceId).To(Equal(spaceId2))
			})
		})
	})
})
