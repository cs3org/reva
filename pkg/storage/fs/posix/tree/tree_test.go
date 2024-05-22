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
		Describe("of files", func() {
			It("handles new files", func() {
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

			It("handles changed files", func() {
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

			It("handles deleted files", func() {
				_, err := os.Create(env.Root + "/users/" + env.Owner.Username + "/deleted.txt")
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/deleted.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())

				Expect(os.Remove(env.Root + "/users/" + env.Owner.Username + "/deleted.txt")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/deleted.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())
			})

			It("handles moved files", func() {
				// Create empty file
				_, err := os.Create(env.Root + "/users/" + env.Owner.Username + "/original.txt")
				Expect(err).ToNot(HaveOccurred())

				fileID := ""
				// Wait for the file to be indexed
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/original.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					fileID = n.ID
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())

				// Move file
				Expect(os.Rename(env.Root+"/users/"+env.Owner.Username+"/original.txt", env.Root+"/users/"+env.Owner.Username+"/moved.txt")).To(Succeed())

				// Wait for the file to be indexed
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/original.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/moved.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).To(Equal(fileID))
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())
			})
		})

		Describe("of directories", func() {
			It("handles new directories", func() {
				Expect(os.Mkdir(env.Root+"/users/"+env.Owner.Username+"/assimilated", 0700)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/assimilated",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())
			})

			It("handles files in directories", func() {
				Expect(os.Mkdir(env.Root+"/users/"+env.Owner.Username+"/assimilated", 0700)).To(Succeed())
				Expect(os.WriteFile(env.Root+"/users/"+env.Owner.Username+"/assimilated/file.txt", []byte("hello world"), 0600)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,

						Path: "/assimilated",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
				}).Should(Succeed())
			})

			It("handles deleted directories", func() {
				Expect(os.Mkdir(env.Root+"/users/"+env.Owner.Username+"/deleted", 0700)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/deleted",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())

				Expect(os.Remove(env.Root + "/users/" + env.Owner.Username + "/deleted")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/deleted",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())
			})

			It("handles moved directories", func() {
				Expect(os.Mkdir(env.Root+"/users/"+env.Owner.Username+"/original", 0700)).To(Succeed())
				Expect(os.WriteFile(env.Root+"/users/"+env.Owner.Username+"/original/file.txt", []byte("hello world"), 0600)).To(Succeed())

				dirId := ""
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/original",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
					dirId = n.ID
				}).Should(Succeed())

				Expect(os.Rename(env.Root+"/users/"+env.Owner.Username+"/original", env.Root+"/users/"+env.Owner.Username+"/moved")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/original",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       "/moved",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Exists).To(BeTrue())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).To(Equal(dirId))
					g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
				}).Should(Succeed())
			})
		})
	})

	Describe("propagation", func() {
		It("propagates new files in a directory", func() {
			Expect(os.Mkdir(env.Root+"/users/"+env.Owner.Username+"/assimilated", 0700)).To(Succeed())
			time.Sleep(100 * time.Millisecond) // Give it some time to settle down
			Expect(os.WriteFile(env.Root+"/users/"+env.Owner.Username+"/assimilated/file.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
			}).Should(Succeed())

			Expect(os.WriteFile(env.Root+"/users/"+env.Owner.Username+"/assimilated/file2.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(22)))
			}).Should(Succeed())
		})
	})
})
