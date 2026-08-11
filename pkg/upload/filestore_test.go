package upload

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/v2/pkg/handler"
)

func nopLog() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

var _ = Describe("FileStore", func() {
	var (
		ctx       context.Context
		uploadDir string
		fs        *FileStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		uploadDir = filepath.Join(GinkgoT().TempDir(), "uploads")
		fs = NewFileStore(uploadDir, TokenOptions{}, nopLog())
	})

	Describe("Setup", func() {
		It("creates the upload directory", func() {
			Expect(fs.Setup()).To(Succeed())

			info, err := os.Stat(uploadDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		})

		It("is idempotent", func() {
			Expect(fs.Setup()).To(Succeed())
			Expect(fs.Setup()).To(Succeed())
		})
	})

	Describe("New", func() {
		BeforeEach(func() {
			Expect(fs.Setup()).To(Succeed())
		})

		It("allocates a non-empty id", func() {
			Expect(fs.New(ctx).ID()).ToNot(BeEmpty())
		})

		It("allocates unique ids", func() {
			Expect(fs.New(ctx).ID()).ToNot(Equal(fs.New(ctx).ID()))
		})

		It("stamps the storage type", func() {
			info, err := fs.New(ctx).GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			// Deliberately OcisStore's value: sessions written by either store must stay
			// readable across a rolling deploy.
			Expect(info.Storage["Type"]).To(Equal("OCISStore"))
		})

		It("initialises the metadata map", func() {
			info, err := fs.New(ctx).GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.MetaData).ToNot(BeNil())
		})
	})

	Describe("Get", func() {
		BeforeEach(func() {
			Expect(fs.Setup()).To(Succeed())
		})

		It("loads a persisted session", func() {
			s := fs.New(ctx).(*FileSession)
			Expect(s.TouchBin()).To(Succeed())
			Expect(s.Persist(ctx)).To(Succeed())

			got, err := fs.Get(ctx, s.ID())
			Expect(err).ToNot(HaveOccurred())
			Expect(got.ID()).To(Equal(s.ID()))

			info, err := got.GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.Storage["Type"]).To(Equal("OCISStore"))
		})

		It("takes the offset from the staged binary size", func() {
			s := fs.New(ctx).(*FileSession)
			Expect(s.TouchBin()).To(Succeed())
			Expect(s.Persist(ctx)).To(Succeed())

			payload := []byte("hello world")
			Expect(os.WriteFile(s.binPath(), payload, 0600)).To(Succeed())

			got, err := fs.Get(ctx, s.ID())
			Expect(err).ToNot(HaveOccurred())
			Expect(got.Offset()).To(Equal(int64(len(payload))))
		})

		It("reports a missing info file as not found", func() {
			_, err := fs.Get(ctx, "no-such-id")
			Expect(err).To(MatchError(tusd.ErrNotFound))
		})

		It("reports corrupt info as an error, not as not found", func() {
			s := fs.New(ctx).(*FileSession)
			Expect(s.TouchBin()).To(Succeed())
			Expect(s.Persist(ctx)).To(Succeed())

			// Overwrite the .info with garbage JSON.
			Expect(os.WriteFile(s.infoPath(), []byte("{not valid json"), 0600)).To(Succeed())

			_, err := fs.Get(ctx, s.ID())
			Expect(err).To(HaveOccurred())
			Expect(err).ToNot(MatchError(tusd.ErrNotFound))
		})

		It("reports a missing staged binary as not found", func() {
			s := fs.New(ctx).(*FileSession)
			// Write .info but do NOT create the .bin file.
			Expect(s.Persist(ctx)).To(Succeed())

			_, err := fs.Get(ctx, s.ID())
			Expect(err).To(MatchError(tusd.ErrNotFound))
		})
	})

	Describe("List", func() {
		BeforeEach(func() {
			Expect(fs.Setup()).To(Succeed())
		})

		It("is empty on a fresh store", func() {
			sessions, err := fs.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(sessions).To(BeEmpty())
		})

		It("returns every persisted session", func() {
			s1 := fs.New(ctx).(*FileSession)
			Expect(s1.TouchBin()).To(Succeed())
			Expect(s1.Persist(ctx)).To(Succeed())

			s2 := fs.New(ctx).(*FileSession)
			Expect(s2.TouchBin()).To(Succeed())
			Expect(s2.Persist(ctx)).To(Succeed())

			sessions, err := fs.List(ctx)
			Expect(err).ToNot(HaveOccurred())

			ids := make([]string, 0, len(sessions))
			for _, s := range sessions {
				ids = append(ids, s.ID())
			}
			Expect(ids).To(ConsistOf(s1.ID(), s2.ID()))
		})

		It("skips a session whose staged binary is gone", func() {
			good := fs.New(ctx).(*FileSession)
			Expect(good.TouchBin()).To(Succeed())
			Expect(good.Persist(ctx)).To(Succeed())

			bad := fs.New(ctx).(*FileSession)
			Expect(bad.TouchBin()).To(Succeed())
			Expect(bad.Persist(ctx)).To(Succeed())
			Expect(os.Remove(bad.binPath())).To(Succeed())

			sessions, err := fs.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(sessions).To(HaveLen(1))
			Expect(sessions[0].ID()).To(Equal(good.ID()))
		})
	})
})

var _ = Describe("FileStoreFromDriverConf", func() {
	It("returns nil for a nil config", func() {
		Expect(FileStoreFromDriverConf(nil, nopLog())).To(BeNil())
	})

	// root is a storage root, so uploads are staged in a subdirectory of it. This
	// is the layout OcisStore uses (decomposedfs.go:258 joins o.Root itself).
	It("stages uploads below the root key", func() {
		root := GinkgoT().TempDir()
		fs := FileStoreFromDriverConf(map[string]interface{}{"root": root}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(filepath.Join(root, "uploads")))
	})

	// storage_root is the ocm driver's spelling of root, same layout
	// (ocm/storage/received/upload.go:212).
	It("stages uploads below the storage_root key", func() {
		root := GinkgoT().TempDir()
		fs := FileStoreFromDriverConf(map[string]interface{}{"storage_root": root}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(filepath.Join(root, "uploads")))
	})

	// upload_directory already names the upload directory, the way decomposedfs
	// reads it (options.go:172, posix tree.go:168), so it must not be joined again.
	It("takes upload_directory as the upload directory itself", func() {
		dir := GinkgoT().TempDir()
		fs := FileStoreFromDriverConf(map[string]interface{}{"upload_directory": dir}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(dir))
	})

	It("prefers upload_directory over root", func() {
		root := GinkgoT().TempDir()
		uploadDir := GinkgoT().TempDir()
		fs := FileStoreFromDriverConf(map[string]interface{}{
			"root":             root,
			"upload_directory": uploadDir,
		}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(uploadDir))
	})

	It("returns nil when no root key is present", func() {
		Expect(FileStoreFromDriverConf(map[string]interface{}{"some_other_key": "value"}, nopLog())).To(BeNil())
	})
})

var _ = Describe("NewFileStoreFromConfig", func() {
	// The service-level value already names the upload directory, so it is used
	// verbatim rather than joined.
	It("uses the service-level upload dir when set", func() {
		uploadDir := GinkgoT().TempDir()
		fs := NewFileStoreFromConfig(uploadDir, map[string]interface{}{"root": "/ignored"}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(uploadDir))
	})

	It("falls back to the driver config", func() {
		root := GinkgoT().TempDir()
		fs := NewFileStoreFromConfig("", map[string]interface{}{"root": root}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(filepath.Join(root, "uploads")))
	})

	It("returns nil when neither source resolves", func() {
		Expect(NewFileStoreFromConfig("", nil, nopLog())).To(BeNil())
	})

	// A service-level upload directory must not drop the driver's tokens: they sign
	// the transfer URL postprocessing downloads the staged bytes from.
	It("keeps the driver tokens when the upload dir comes from the service", func() {
		uploadDir := GinkgoT().TempDir()
		fs := NewFileStoreFromConfig(uploadDir, map[string]interface{}{
			"root": "/ignored",
			"tokens": map[string]interface{}{
				"transfer_shared_secret": "s3cret",
				"download_endpoint":      "https://dl.example.com/data/",
			},
		}, nopLog())

		Expect(fs).ToNot(BeNil())
		Expect(fs.uploadDir).To(Equal(uploadDir))
		Expect(fs.opts.TransferSharedSecret).To(Equal("s3cret"))
		Expect(fs.opts.DownloadEndpoint).To(Equal("https://dl.example.com/data/"))
	})
})
