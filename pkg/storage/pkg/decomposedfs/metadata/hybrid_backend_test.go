package metadata_test

import (
	"context"
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencloud-eu/reva/v2/pkg/storage/cache"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/prefixes"
	"github.com/pkg/xattr"
	"github.com/shamaton/msgpack/v2"
)

var _ = Describe("HybridBackend", func() {
	var (
		tmpdir string
		n      testNode

		backend metadata.Backend

		keySmall   = prefixes.GrantUserAcePrefix + "1"
		dataSmall  = []byte("1")
		keySmall2  = prefixes.GrantUserAcePrefix + "2"
		dataSmall2 = []byte("2")
		keyBig     = prefixes.GrantUserAcePrefix + "100"
		dataBig    = []byte("fooooooothosearetoomanybytes")
	)

	BeforeEach(func() {
		var err error
		tmpdir, err = os.MkdirTemp(os.TempDir(), "HybridBackendTest-")
		Expect(err).ToNot(HaveOccurred())

		offloadLimit := len(prefixes.GrantUserAcePrefix) + 2
		backend = metadata.NewHybridBackend(offloadLimit,
			func(n metadata.MetadataNode) string {
				return n.InternalPath() + ".mpk"
			},
			cache.Config{
				Database: tmpdir,
			})
	})

	JustBeforeEach(func() {
		n = testNode{
			spaceID: "123",
			id:      "456",
			path:    path.Join(tmpdir, "file"),
		}
		_, err := os.Create(n.InternalPath())
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("Set", func() {
		It("sets an attribute", func() {
			data := []byte(`bar\`)
			err := backend.Set(context.Background(), n, "user.foo", data)
			Expect(err).ToNot(HaveOccurred())

			readData, err := backend.Get(context.Background(), n, "user.foo")
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(data))
		})

		It("doesn't offload grants if size is not exceeded", func() {
			err := backend.Set(context.Background(), n, keySmall, dataSmall)
			Expect(err).ToNot(HaveOccurred())

			readData, err := xattr.Get(n.InternalPath(), keySmall)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(dataSmall))

			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).To(HaveOccurred())
		})

		It("offloads grants if size is exceeded", func() {
			err := backend.Set(context.Background(), n, keyBig, dataBig)
			Expect(err).ToNot(HaveOccurred())

			By("not adding the grant to the xattrs")
			_, err = xattr.Get(n.InternalPath(), keyBig)
			Expect(err).To(HaveOccurred())

			By("adding the grant to the messagepack file")
			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			msgBytes, err := os.ReadFile(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			attribs := map[string][]byte{}
			err = msgpack.Unmarshal(msgBytes, &attribs)
			Expect(err).ToNot(HaveOccurred())
			Expect(attribs[keyBig]).To(Equal(dataBig))
		})

		It("offloads when the written attribute is small but the total exceeds the size", func() {
			err := backend.Set(context.Background(), n, keySmall, dataSmall)
			Expect(err).ToNot(HaveOccurred())
			err = backend.Set(context.Background(), n, keySmall2, dataSmall2)
			Expect(err).ToNot(HaveOccurred())

			By("not adding the grant to the xattrs")
			_, err = xattr.Get(n.InternalPath(), keySmall)
			Expect(err).To(HaveOccurred())
			_, err = xattr.Get(n.InternalPath(), keySmall2)
			Expect(err).To(HaveOccurred())

			By("adding the grant to the messagepack file")
			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			msgBytes, err := os.ReadFile(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			attribs := map[string][]byte{}
			err = msgpack.Unmarshal(msgBytes, &attribs)
			Expect(err).ToNot(HaveOccurred())
			Expect(attribs[keySmall]).To(Equal(dataSmall))
			Expect(attribs[keySmall2]).To(Equal(dataSmall2))
		})

		It("offloads existing grants when offloading", func() {
			// The first grant will not trigger offloading
			err := backend.Set(context.Background(), n, keySmall, dataSmall)
			Expect(err).ToNot(HaveOccurred())
			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).To(HaveOccurred())

			By("triggering offloading")
			err = backend.Set(context.Background(), n, keyBig, dataBig)
			Expect(err).ToNot(HaveOccurred())

			By("Removing all grants from the xattrs")
			_, err = xattr.Get(n.InternalPath(), keySmall)
			Expect(err).To(HaveOccurred())
			_, err = xattr.Get(n.InternalPath(), keyBig)
			Expect(err).To(HaveOccurred())

			By("adding all grants to the messagepack file")
			_, err = os.Stat(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			msgBytes, err := os.ReadFile(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			attribs := map[string][]byte{}
			err = msgpack.Unmarshal(msgBytes, &attribs)
			Expect(err).ToNot(HaveOccurred())
			Expect(attribs[keySmall]).To(Equal(dataSmall))
			Expect(attribs[keyBig]).To(Equal(dataBig))
		})
	})

	Describe("Get", func() {
		Context("with unoffloaded grants", func() {
			JustBeforeEach(func() {
				err := backend.Set(context.Background(), n, keySmall, dataSmall)
				Expect(err).ToNot(HaveOccurred())
			})

			It("reads the grants", func() {
				readData, err := backend.Get(context.Background(), n, keySmall)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataSmall))
			})
		})

		Context("with offloaded grants", func() {
			JustBeforeEach(func() {
				err := backend.SetMultiple(context.Background(), n, map[string][]byte{
					keySmall: dataSmall,
					keyBig:   dataBig,
				}, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("reads the grants", func() {
				readData, err := backend.Get(context.Background(), n, keySmall)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataSmall))

				readData, err = backend.Get(context.Background(), n, keyBig)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataBig))
			})
		})
	})
})
