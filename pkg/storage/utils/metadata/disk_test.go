package metadata_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
)

func TestDisk(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disk Suite")
}

var _ = Describe("Disk", func() {
	var (
		ctx     context.Context
		storage metadata.Storage
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		storage, err = metadata.NewDiskStorage(GinkgoT().TempDir())
		Expect(err).ToNot(HaveOccurred())
		Expect(storage.Init(ctx, "test")).To(Succeed())
	})

	Describe("Upload", func() {
		It("returns AlreadyExists on IfNoneMatch=* when file exists", func() {
			Expect(storage.SimpleUpload(ctx, "f", []byte("v1"))).To(Succeed())
			_, err := storage.Upload(ctx, metadata.UploadRequest{
				Path:        "f",
				Content:     []byte("v2"),
				IfNoneMatch: []string{"*"},
			})
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.AlreadyExists)
			Expect(ok).To(BeTrue())
		})
	})

	Describe("Download", func() {
		It("returns NotModified when IfNoneMatch etag matches", func() {
			res, err := storage.Upload(ctx, metadata.UploadRequest{Path: "f", Content: []byte("v1")})
			Expect(err).ToNot(HaveOccurred())
			_, err = storage.Download(ctx, metadata.DownloadRequest{
				Path:        "f",
				IfNoneMatch: []string{res.Etag},
			})
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.NotModified)
			Expect(ok).To(BeTrue())
		})
	})
})
