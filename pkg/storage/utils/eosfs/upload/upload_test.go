package upload_test

import (
	"bytes"
	"context"
	"os"
	"path"

	eosclientmocks "github.com/cs3org/reva/v2/pkg/eosclient/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/eosfs/upload"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/test-go/testify/mock"
	tusd "github.com/tus/tusd/pkg/handler"
)

var _ = Describe("upload", func() {
	var (
		storageRoot string
		uploadID    = "uploadid"
		ctx         context.Context
		client      *eosclientmocks.EOSClient
	)

	BeforeEach(func() {
		var err error
		storageRoot, err = os.MkdirTemp("", "eosfs-upload-test")
		Expect(err).ToNot(HaveOccurred())

		client = &eosclientmocks.EOSClient{}
		ctx = context.Background()
	})

	AfterEach(func() {
		if storageRoot != "" {
			os.RemoveAll(storageRoot)
		}
	})

	Describe("New", func() {
		It("creates a new Upload", func() {
			ui, err := upload.New(ctx, tusd.FileInfo{
				ID: uploadID,
			}, storageRoot, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(ui).ToNot(BeNil())
			Expect(ui.ID).To(Equal(uploadID))

			_, err = os.Stat(path.Join(storageRoot, uploadID+".info"))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Get", func() {
		It("returns the upload", func() {
			_, err := upload.New(ctx, tusd.FileInfo{
				ID: uploadID,
			}, storageRoot, client)
			Expect(err).ToNot(HaveOccurred())

			ul, err := upload.Get(ctx, uploadID, storageRoot, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(ul).ToNot(BeNil())
			info, err := ul.GetInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.ID).To(Equal(uploadID))
		})
	})

	Describe("Upload", func() {
		var (
			u *upload.Upload
		)

		BeforeEach(func() {
			var err error
			u, err = upload.New(ctx, tusd.FileInfo{
				ID: uploadID,
			}, storageRoot, client)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("GetInfo", func() {
			It("returns the info", func() {
				info, err := u.GetInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(info).ToNot(BeNil())
				Expect(info.ID).To(Equal(uploadID))
			})
		})

		Describe("WriteChunk", func() {
			It("writes a chunk to the storage", func() {
				r := bytes.NewBufferString("12345")
				written, err := u.WriteChunk(ctx, 0, r)
				Expect(err).ToNot(HaveOccurred())
				Expect(written).To(Equal(int64(5)))
			})
		})

		Describe("FinishUpload", func() {
			It("uploads the file to the storage", func() {
				client.On("Write", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				r := bytes.NewBufferString("12345")
				written, err := u.WriteChunk(ctx, 0, r)
				Expect(err).ToNot(HaveOccurred())
				Expect(written).To(Equal(int64(5)))

				err = u.FinishUpload(ctx)
				Expect(err).ToNot(HaveOccurred())

				client.AssertNumberOfCalls(GinkgoT(), "Write", 1)
			})
		})

		Describe("Terminate", func() {
			It("cleans up", func() {
				exists := func(path string) bool {
					_, err := os.Stat(path)
					return err == nil
				}
				Expect(exists(u.BinPath)).To(BeTrue())
				Expect(exists(u.InfoPath)).To(BeTrue())

				Expect(u.Terminate(ctx)).To(Succeed())

				Expect(exists(u.BinPath)).To(BeFalse())
				Expect(exists(u.InfoPath)).To(BeFalse())
			})
		})
	})
})
