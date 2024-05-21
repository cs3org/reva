package tree_test

import (
	"os"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	helpers "github.com/cs3org/reva/v2/pkg/storage/fs/posix/testhelpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tree", func() {
	var (
		env *helpers.TestEnv
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		// wait for the inotify watcher to start
		time.Sleep(1 * time.Second)
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("assimilation", func() {
		It("picks up new files", func() {
			_, err := os.Create(env.Root + "/users/" + env.Owner.Username + "/assimilated.txt")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/assimilated.txt",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.Blobsize).To(Equal(int64(0)))
			}).WithTimeout(1 * time.Second).ProbeEvery(200 * time.Millisecond).Should(Succeed())
		})

		It("picks up changed files", func() {
			// Create empty file
			f, err := os.Create(env.Root + "/users/" + env.Owner.Username + "/changed.txt")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/changed.txt",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.Blobsize).To(Equal(int64(0)))
			}).WithTimeout(1 * time.Second).ProbeEvery(200 * time.Millisecond).Should(Succeed())

			// Change file content
			_, err = f.Write([]byte("hello world"))
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Close()).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/changed.txt",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.Blobsize).To(Equal(int64(11)))
			}).Should(Succeed())
		})
	})
})
