package migrator_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/migrator"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
)

var _ = Describe("Migrator", func() {
	var (
		env *helpers.TestEnv

		nullLogger = zerolog.New(ioutil.Discard).With().Logger()
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("migrating metadata - migration 0003", func() {
		When("staying at xattrs", func() {
			JustBeforeEach(func() {
				var err error
				env, err = helpers.NewTestEnv(map[string]interface{}{
					"metadata_backend": "xattrs",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("doesn't migrate", func() {
				m := migrator.New(env.Lookup, &nullLogger)
				err := m.RunMigrations()
				Expect(err).ToNot(HaveOccurred())

				nRef, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())
				nameAttr, err := xattr.Get(nRef.InternalPath(), prefixes.NameAttr)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(nameAttr)).To(Equal("file1"))

				_, err = os.Stat(nRef.InternalPath() + ".mpk")
				Expect(err).To(HaveOccurred())
			})
		})

		When("going from xattrs to messagepack", func() {
			var (
				path    string
				backend metadata.Backend
			)

			JustBeforeEach(func() {
				backend = metadata.NewMessagePackBackend(env.Root, env.Options.FileMetadataCache)

				var err error
				env, err = helpers.NewTestEnv(map[string]interface{}{
					"metadata_backend": "xattrs",
				})
				Expect(err).ToNot(HaveOccurred())

				nRef, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
					ResourceId: env.SpaceRootRes,
					Path:       "/dir1/file1",
				})
				Expect(err).ToNot(HaveOccurred())
				path = nRef.InternalPath()

				// Change backend to messagepack
				env.Lookup = lookup.New(backend, env.Options)
			})

			It("migrates", func() {
				m := migrator.New(env.Lookup, &nullLogger)
				err := m.RunMigrations()
				Expect(err).ToNot(HaveOccurred())

				nameAttr, _ := xattr.Get(path, prefixes.NameAttr)
				Expect(nameAttr).To(Equal([]byte{}))

				_, err = os.Stat(path + ".mpk")
				Expect(err).ToNot(HaveOccurred())
				nameAttr, err = backend.Get(path, prefixes.NameAttr)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(nameAttr)).To(Equal("file1"))
			})
		})
	})
})
