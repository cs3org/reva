package lookup_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/test-go/testify/mock"

	mocks "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/lookup/mocks"
	helpers "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/testhelpers"
)

var _ = Describe("Lookup", func() {
	var (
		env       *helpers.TestEnv
		mockCache *mocks.IDCache

		spaceID string
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(map[string]any{})
		Expect(err).ToNot(HaveOccurred())

		mockCache = mocks.NewIDCache(GinkgoT())
		env.Lookup.IDCache = mockCache

		mockCache.EXPECT().GetByPath(mock.Anything, mock.Anything).Return("", "", false)
		spaceID, err = env.Lookup.GenerateSpaceID("personal", env.Owner)
		Expect(err).ToNot(HaveOccurred())
		Expect(spaceID).ToNot(BeEmpty())
	})

	Describe("GenerateSpaceId", func() {
		Context("with project spaces", func() {
			It("should generate a project space id", func() {
				spaceID, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceID).ToNot(BeEmpty())
			})

			It("should not reuse existing ids", func() {
				spaceID, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceID).ToNot(BeEmpty())

				spaceID2, err := env.Lookup.GenerateSpaceID("project", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceID2).ToNot(BeEmpty())
				Expect(spaceID).ToNot(Equal(spaceID2))
			})
		})

		Context("with personal spaces", func() {
			It("should reuse existing ids", func() {
				spaceID2, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceID2).ToNot(BeEmpty())
				Expect(spaceID).To(Equal(spaceID2))
			})

			It("should not generate a new id but pick up the one from disk instead when reading from the cache fails", func() {
				mockCache.EXPECT().GetByPath(mock.Anything, mock.Anything).Return("", "", false)

				spaceID2, err := env.Lookup.GenerateSpaceID("personal", env.Owner)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaceID2).ToNot(BeEmpty())
				Expect(spaceID).To(Equal(spaceID2))
			})
		})
	})

	Describe("LockfilePaths", func() {
		It("returns the lock file paths for a given node", func() {
			nodeID := "node-1"
			spaceRoot := "/path/to/space"

			mockCache.EXPECT().Get(mock.Anything, spaceID, spaceID).Return(spaceRoot, true)
			mockCache.EXPECT().Get(mock.Anything, spaceID, nodeID).Return(filepath.Join(spaceRoot, "file"), true)

			lockPaths := env.Lookup.LockfilePaths(spaceID, nodeID)
			Expect(lockPaths).To(HaveLen(2))
			Expect(lockPaths[0]).To(Equal("/path/to/space/.oc-nodes/no/de/-1.lock"))
			Expect(lockPaths[1]).To(Equal("/path/to/space/file.lock"))
		})

		It("returns an empty array if the internal path cannot be found", func() {
			nodeID := "node-that-does-not-exist"

			mockCache.EXPECT().Get(mock.Anything, spaceID, spaceID).Return("", false)

			lockPaths := env.Lookup.LockfilePaths(spaceID, nodeID)
			Expect(lockPaths).To(BeEmpty())
		})
	})
})
