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
	"github.com/vmihailenco/msgpack/v5"
)

var _ = Describe("HybridBackend", func() {
	var (
		tmpdir string
		n      testNode

		backend metadata.Backend

		keySmall          = prefixes.GrantUserAcePrefix + "1"
		dataSmall         = []byte("1")
		keySmall2         = prefixes.MetadataPrefix + "2"
		dataSmall2        = []byte("2")
		keyBig            = prefixes.GrantUserAcePrefix + "100"
		dataBig           = []byte("fooooooothosearetoomanybytes")
		nonOffloadingKey  = "user.foo"
		nonOffloadingData = []byte("bar")
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

		It("doesn't offload metadata if size is not exceeded", func() {
			err := backend.Set(context.Background(), n, keySmall, dataSmall)
			Expect(err).ToNot(HaveOccurred())

			readData, err := xattr.Get(n.InternalPath(), keySmall)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(dataSmall))

			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).To(HaveOccurred())
		})

		It("offloads metadata if size is exceeded", func() {
			err := backend.Set(context.Background(), n, keyBig, dataBig)
			Expect(err).ToNot(HaveOccurred())

			By("not adding the metadata to the xattrs")
			_, err = xattr.Get(n.InternalPath(), keyBig)
			Expect(err).To(HaveOccurred())

			By("adding the metadata to the messagepack file")
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

			By("not adding the metadata to the xattrs")
			_, err = xattr.Get(n.InternalPath(), keySmall)
			Expect(err).To(HaveOccurred())
			_, err = xattr.Get(n.InternalPath(), keySmall2)
			Expect(err).To(HaveOccurred())

			By("adding the metadata to the messagepack file")
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

		It("offloads existing metadata as well when offloading", func() {
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

		It("still writes non-offloading metadata to the xattrs, even when offloading", func() {
			err := backend.Set(context.Background(), n, keyBig, dataBig)
			Expect(err).ToNot(HaveOccurred())
			err = backend.Set(context.Background(), n, nonOffloadingKey, nonOffloadingData)
			Expect(err).ToNot(HaveOccurred())

			By("not adding the offloading key to the xattrs")
			_, err = xattr.Get(n.InternalPath(), keyBig)
			Expect(err).To(HaveOccurred())
			By("adding the non-offloading key to the xattrs")
			b, err := xattr.Get(n.InternalPath(), nonOffloadingKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(Equal(nonOffloadingData))

			By("adding the metadata to the messagepack file")
			messagepackPath := backend.MetadataPath(n)
			_, err = os.Stat(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			msgBytes, err := os.ReadFile(messagepackPath)
			Expect(err).ToNot(HaveOccurred())

			attribs := map[string][]byte{}
			err = msgpack.Unmarshal(msgBytes, &attribs)
			Expect(err).ToNot(HaveOccurred())
			Expect(attribs[keyBig]).To(Equal(dataBig))

			By("not adding the non-offloading metadata to the messagepack file")
			Expect(attribs[nonOffloadingKey]).To(BeEmpty())
		})
	})

	Describe("Get", func() {
		Context("with unoffloaded grants", func() {
			JustBeforeEach(func() {
				err := backend.Set(context.Background(), n, keySmall, dataSmall)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns ENOATTR if the requested non-offloading attribute does not exist", func() {
				_, err := backend.Get(context.Background(), n, "user.oc.idonotexist")
				Expect(err).To(HaveOccurred())
				Expect(err.(*xattr.Error).Err).To(Equal(xattr.ENOATTR))
			})

			It("reads the grants", func() {
				readData, err := backend.Get(context.Background(), n, keySmall)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataSmall))
			})
		})

		Context("with offloaded metadata", func() {
			JustBeforeEach(func() {
				err := backend.SetMultiple(context.Background(), n, map[string][]byte{
					keySmall:         dataSmall,
					keyBig:           dataBig,
					nonOffloadingKey: nonOffloadingData,
				}, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns ENOATTR if the requested offloading attribute does not exist", func() {
				_, err := backend.Get(context.Background(), n, "user.oc.md.idonotexist")
				Expect(err).To(HaveOccurred())
				Expect(err.(*xattr.Error).Err).To(Equal(xattr.ENOATTR))
			})

			It("reads offloaded metadata", func() {
				readData, err := backend.Get(context.Background(), n, keySmall)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataSmall))

				readData, err = backend.Get(context.Background(), n, keyBig)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(dataBig))
			})

			It("reads non-offloaded metadata", func() {
				readData, err := backend.Get(context.Background(), n, nonOffloadingKey)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(nonOffloadingData))
			})
		})
	})

	Describe("Remove", func() {
		It("removes non-offloaded metadata", func() {
			err := backend.Set(context.Background(), n, nonOffloadingKey, nonOffloadingData)
			Expect(err).ToNot(HaveOccurred())
			readData, err := backend.Get(context.Background(), n, nonOffloadingKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(nonOffloadingData))

			err = backend.Remove(context.Background(), n, nonOffloadingKey, true)
			Expect(err).ToNot(HaveOccurred())

			_, err = backend.Get(context.Background(), n, nonOffloadingKey)
			Expect(err).To(HaveOccurred())
		})

		It("removes offloaded metadata", func() {
			err := backend.Set(context.Background(), n, keyBig, dataBig)
			Expect(err).ToNot(HaveOccurred())
			readData, err := backend.Get(context.Background(), n, keyBig)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(dataBig))

			err = backend.Remove(context.Background(), n, keyBig, true)
			Expect(err).ToNot(HaveOccurred())

			_, err = backend.Get(context.Background(), n, keyBig)
			Expect(err).To(HaveOccurred())
		})

		It("removes non-offloaded metadata in the offloading case", func() {
			err := backend.SetMultiple(context.Background(), n, map[string][]byte{
				keySmall:         dataSmall,
				keyBig:           dataBig,
				nonOffloadingKey: nonOffloadingData,
			}, false)
			Expect(err).ToNot(HaveOccurred())
			readData, err := backend.Get(context.Background(), n, keyBig)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(dataBig))
			readData, err = backend.Get(context.Background(), n, nonOffloadingKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(readData).To(Equal(nonOffloadingData))

			err = backend.Remove(context.Background(), n, nonOffloadingKey, true)
			Expect(err).ToNot(HaveOccurred())

			_, err = backend.Get(context.Background(), n, nonOffloadingKey)
			Expect(err).To(HaveOccurred())
		})
	})
})
