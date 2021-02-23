package decomposedfs_test

import (
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lookup", func() {
	var (
		env *helpers.TestEnv
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("Path", func() {
		It("returns the path including a leading slash", func() {
			n, err := env.Lookup.NodeFromPath(env.Ctx, "/dir1/file1")
			Expect(err).ToNot(HaveOccurred())

			path, err := env.Lookup.Path(env.Ctx, n)
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/file1"))
		})
	})
})
