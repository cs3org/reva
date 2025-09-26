package tree_test

import (
	"crypto/rand"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	helpers "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/testhelpers"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/prefixes"
	"github.com/shirou/gopsutil/process"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsetLength := len(charset)

	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	for i := 0; i < length; i++ {
		randomBytes[i] = charset[int(randomBytes[i])%charsetLength]
	}

	return string(randomBytes), nil
}

var (
	env *helpers.TestEnv

	root string
)

var _ = SynchronizedBeforeSuite(func() {
	var err error
	env, err = helpers.NewTestEnv(map[string]interface{}{"watch_fs": true})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		// Get all running processes
		processes, err := process.Processes()
		if err != nil {
			panic("could not get processes: " + err.Error())
		}

		// Search for the process named "inotifywait"
		for _, p := range processes {
			name, err := p.Name()
			if err != nil {
				log.Println(err)
				continue
			}

			if strings.Contains(name, "inotifywait") {
				// Give it some time to setup the watches
				time.Sleep(2 * time.Second)
				return true
			}
		}
		return false
	}).Should(BeTrue())
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	if env != nil {
		env.Cleanup()
	}
})

var _ = Describe("Tree", func() {
	var (
		subtree string
	)

	BeforeEach(func() {
		SetDefaultEventuallyTimeout(15 * time.Second)

		var err error
		subtree, err = generateRandomString(10)
		Expect(err).ToNot(HaveOccurred())
		subtree = "/" + subtree
		root = env.Root + "/users/" + env.Owner.Username + subtree
		Expect(os.Mkdir(root, 0700)).To(Succeed())

		Eventually(func(g Gomega) {
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       subtree,
			})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(n.Exists).To(BeTrue())
		}).Should(Succeed())
	})

	Describe("assimilation", func() {
		Describe("of files", func() {
			It("handles new files", func() {
				_, err := os.Create(root + "/assimilated.txt")
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/assimilated.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.Blobsize).To(Equal(int64(0)))
					g.Expect(n.Xattr(env.Ctx, prefixes.ChecksumPrefix+"adler32")).ToNot(BeEmpty())
				}).ProbeEvery(200 * time.Millisecond).Should(Succeed())
			})

			It("does not ignore .lock files", func() {
				_, err := os.Create(root + "/Composer.lock")
				Expect(err).ToNot(HaveOccurred())

				Consistently(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/Composer.lock",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
					g.Expect(n.ID).To(BeEmpty())
				}).ProbeEvery(200 * time.Millisecond).Should(Succeed())
			})

			It("handles new files which are still being written", func() {
				f, err := os.Create(root + "/file.txt")
				Expect(err).ToNot(HaveOccurred())

				// Write initial bytes
				initialContent := []byte("initial data")
				_, err = f.Write(initialContent)
				Expect(err).ToNot(HaveOccurred())
				err = f.Sync()
				Expect(err).ToNot(HaveOccurred())

				By("assimilating the file with the initial content")
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/file.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.Blobsize).To(Equal(int64(len(initialContent))))
					g.Expect(n.Xattr(env.Ctx, prefixes.ChecksumPrefix+"adler32")).ToNot(BeEmpty())
				}).ProbeEvery(200 * time.Millisecond).Should(Succeed())

				// Write more data to the file
				additionalContent := []byte(" and more data")
				_, err = f.Write(additionalContent)
				Expect(err).ToNot(HaveOccurred())

				// Close the file
				err = f.Close()
				Expect(err).ToNot(HaveOccurred())

				By("assimilating the file with the final content")
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/file.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.Blobsize).To(Equal(int64(len(initialContent) + len(additionalContent))))
					checksum, _ := n.Xattr(env.Ctx, prefixes.ChecksumPrefix+"adler32")
					g.Expect(checksum).ToNot(BeEmpty())
				}).Should(Succeed())
			})

			It("handles changed files", func() {
				// Create empty file
				_, err := os.Create(root + "/changed.txt")
				Expect(err).ToNot(HaveOccurred())

				var oldChecksum []byte
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/changed.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.Blobsize).To(Equal(int64(0)))
					oldChecksum, _ = n.Xattr(env.Ctx, prefixes.ChecksumPrefix+"adler32")
					g.Expect(oldChecksum).ToNot(BeEmpty())
				}).ProbeEvery(200 * time.Millisecond).Should(Succeed())

				// Change file content
				Expect(os.WriteFile(root+"/changed.txt", []byte("hello world"), 0600)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/changed.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.Blobsize).To(Equal(int64(11)))
					checksum, _ := n.Xattr(env.Ctx, prefixes.ChecksumPrefix+"adler32")
					g.Expect(checksum).ToNot(Equal(oldChecksum))
				}).Should(Succeed())
			})

			It("handles deleted files", func() {
				_, err := os.Create(root + "/deleted.txt")
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/deleted.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())

				Expect(os.Remove(root + "/deleted.txt")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/deleted.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())
			})

			It("handles moved files", func() {
				// Create empty file
				_, err := os.Create(root + "/original.txt")
				Expect(err).ToNot(HaveOccurred())

				fileID := ""
				// Wait for the file to be indexed
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/original.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					fileID = n.ID
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())

				// Move file
				Expect(os.Rename(root+"/original.txt", root+"/moved.txt")).To(Succeed())

				// Wait for the file to be indexed
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/original.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/moved.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).To(Equal(fileID))
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())
			})

			It("handles id clashes", func() {
				// Create empty file
				_, err := os.Create(root + "/original.txt")
				Expect(err).ToNot(HaveOccurred())

				fileID := ""
				// Wait for the file to be indexed
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/original.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					fileID = n.ID
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())

				// cp file
				cmd := exec.Command("cp", "-a", root+"/original.txt", root+"/moved.txt")
				err = cmd.Run()
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/moved.txt",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.ID).ToNot(Equal(fileID))
					g.Expect(n.Blobsize).To(Equal(int64(0)))
				}).Should(Succeed())
			})
		})

		Describe("of directories", func() {
			It("handles new directories", func() {
				Expect(os.Mkdir(root+"/assimilated", 0700)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/assimilated",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())
			})

			It("handles files in directories", func() {
				Expect(os.Mkdir(root+"/assimilated", 0700)).To(Succeed())
				time.Sleep(100 * time.Millisecond) // Give it some time to settle down
				Expect(os.WriteFile(root+"/assimilated/file.txt", []byte("hello world"), 0600)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/assimilated",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
				}).Should(Succeed())
			})

			It("handles deleted directories", func() {
				Expect(os.Mkdir(root+"/deleted", 0700)).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/deleted",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
				}).Should(Succeed())

				Expect(os.Remove(root + "/deleted")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/deleted",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())
			})

			It("handles moved directories", func() {
				Expect(os.Mkdir(root+"/original", 0700)).To(Succeed())
				time.Sleep(100 * time.Millisecond) // Give it some time to settle down
				Expect(os.WriteFile(root+"/original/file.txt", []byte("hello world"), 0600)).To(Succeed())

				dirId := ""
				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/original",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n).ToNot(BeNil())
					g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
					g.Expect(n.ID).ToNot(BeEmpty())
					g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
					dirId = n.ID
				}).Should(Succeed())

				Expect(os.Rename(root+"/original", root+"/moved")).To(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/original",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(n.Exists).To(BeFalse())
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
						ResourceId: env.SpaceRootRes,
						Path:       subtree + "/moved",
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
			Expect(os.Mkdir(root+"/assimilated", 0700)).To(Succeed())
			time.Sleep(100 * time.Millisecond) // Give it some time to settle down
			Expect(os.WriteFile(root+"/assimilated/file.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree + "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
			}).Should(Succeed())

			Expect(os.WriteFile(root+"/assimilated/file2.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree + "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(22)))
			}).Should(Succeed())
		})

		It("propagates new files in a directory to the parent", func() {
			Expect(env.Tree.WarmupIDCache(env.Root, false, true)).To(Succeed())
			Expect(os.Mkdir(root+"/assimilated", 0700)).To(Succeed())
			time.Sleep(100 * time.Millisecond) // Give it some time to settle down
			Expect(os.WriteFile(root+"/assimilated/file.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree + "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(11)))
			}).Should(Succeed())

			Expect(os.WriteFile(root+"/assimilated/file2.txt", []byte("hello world"), 0600)).To(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree + "/assimilated",
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n).ToNot(BeNil())
				g.Expect(n.Type(env.Ctx)).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				g.Expect(n.ID).ToNot(BeEmpty())
				g.Expect(n.GetTreeSize(env.Ctx)).To(Equal(uint64(22)))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,

					Path: subtree,
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
