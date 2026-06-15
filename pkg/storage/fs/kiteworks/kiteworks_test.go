package kiteworks_test

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/storage/fs/kiteworks"
)

type fixture struct {
	ctx         context.Context
	spaceID     string
	fileID      string
	fileContent string // empty on real box; exact expected content in mock mode
}

func skipIfRealBox() {
	if os.Getenv("KITEWORKS") != "" {
		Skip("mock-only test")
	}
}

func firstFileID(items []*provider.ResourceInfo) string {
	for _, item := range items {
		if item.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
			return item.Id.OpaqueId
		}
	}
	return ""
}

func setupDriver() (storage.FS, *fixture, func()) {
	ep := os.Getenv("KITEWORKS")
	if ep == "" {
		srv := httptest.NewServer(mockKiteworksHandler())
		d, err := kiteworks.New(map[string]interface{}{"endpoint": srv.URL}, nil, nil)
		Expect(err).ToNot(HaveOccurred())
		return d, &fixture{
			ctx:         context.Background(),
			spaceID:     "space-1",
			fileID:      "file-1",
			fileContent: "hello kiteworks",
		}, srv.Close
	}

	d, err := kiteworks.New(map[string]interface{}{"endpoint": ep}, nil, nil)
	Expect(err).ToNot(HaveOccurred())

	ctx := ctxpkg.ContextSetToken(context.Background(), os.Getenv("KITEWORKS_TOKEN"))

	spaces, err := d.ListStorageSpaces(ctx, nil, false)
	Expect(err).ToNot(HaveOccurred(), "real-box ListStorageSpaces failed — check token/endpoint")
	Expect(spaces).ToNot(BeEmpty(), "real box has no top-level folders")

	spaceID := spaces[0].Root.OpaqueId

	children, err := d.ListFolder(ctx, &provider.Reference{
		ResourceId: &provider.ResourceId{SpaceId: spaceID, OpaqueId: spaceID},
	}, nil, nil)
	Expect(err).ToNot(HaveOccurred())

	return d, &fixture{ctx: ctx, spaceID: spaceID, fileID: firstFileID(children)}, func() {}
}

var _ = Describe("kiteworks driver", func() {
	var (
		d    storage.FS
		fix  *fixture
		stop func()
	)

	BeforeEach(func() {
		d, fix, stop = setupDriver()
	})

	AfterEach(func() {
		stop()
	})

	Context("read path", func() {
		Describe("ListStorageSpaces", func() {
			It("returns at least one project space", func() {
				spaces, err := d.ListStorageSpaces(fix.ctx, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaces).ToNot(BeEmpty())
				Expect(spaces[0].SpaceType).To(Equal("project"))
				Expect(spaces[0].Name).ToNot(BeEmpty())
			})

			It("returns space with root ResourceId storageID=kiteworks", func() {
				spaces, err := d.ListStorageSpaces(fix.ctx, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(spaces[0].Root.StorageId).To(Equal("kiteworks"))
			})
		})

		Describe("GetMD", func() {
			It("returns container info for the space root", func() {
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.spaceID},
				}
				ri, err := d.GetMD(fix.ctx, ref, nil, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(ri.Type).To(Equal(provider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(ri.Id.OpaqueId).To(Equal(fix.spaceID))
			})
		})

		Describe("ListFolder", func() {
			It("succeeds for the root space", func() {
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.spaceID},
				}
				_, err := d.ListFolder(fix.ctx, ref, nil, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns expected children in mock mode", func() {
				skipIfRealBox()
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.spaceID},
				}
				infos, err := d.ListFolder(fix.ctx, ref, nil, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(infos).To(HaveLen(1))
				Expect(infos[0].Name).To(Equal("hello.txt"))
				Expect(infos[0].Type).To(Equal(provider.ResourceType_RESOURCE_TYPE_FILE))
			})
		})

		Describe("Download", func() {
			It("streams the file content", func() {
				if fix.fileID == "" {
					Skip("no file found in root space")
				}
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.fileID},
				}
				_, rc, err := d.Download(fix.ctx, ref, func(_ *provider.ResourceInfo) bool { return true })
				Expect(err).ToNot(HaveOccurred())
				Expect(rc).ToNot(BeNil())
				defer rc.Close()
				b, err := io.ReadAll(rc)
				Expect(err).ToNot(HaveOccurred())
				if fix.fileContent != "" {
					Expect(string(b)).To(Equal(fix.fileContent))
				} else {
					Expect(len(b)).To(BeNumerically(">", 0))
				}
			})

			It("returns ResourceInfo without a reader when openReaderFunc returns false", func() {
				if fix.fileID == "" {
					Skip("no file found in root space")
				}
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.fileID},
				}
				ri, rc, err := d.Download(fix.ctx, ref, func(_ *provider.ResourceInfo) bool { return false })
				Expect(err).ToNot(HaveOccurred())
				Expect(ri).ToNot(BeNil())
				Expect(rc).To(BeNil())
			})
		})

		Describe("GetPathByID", func() {
			It("returns the path for the space root", func() {
				id := &provider.ResourceId{StorageId: "kiteworks", SpaceId: fix.spaceID, OpaqueId: fix.spaceID}
				path, err := d.GetPathByID(fix.ctx, id)
				Expect(err).ToNot(HaveOccurred())
				Expect(path).ToNot(BeEmpty())
			})
		})
	})

	Context("error propagation", func() {
		It("GetMD propagates non-404 server errors", func() {
			skipIfRealBox()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{SpaceId: "error-500", OpaqueId: "error-500"},
			}
			_, err := d.GetMD(fix.ctx, ref, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err).ToNot(Satisfy(func(e error) bool { return errors.As(e, new(errtypes.NotFound)) }))
		})

		It("Download resolves root ref where OpaqueId is empty", func() {
			skipIfRealBox()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{SpaceId: fix.spaceID},
			}
			ri, rc, err := d.Download(fix.ctx, ref, func(_ *provider.ResourceInfo) bool { return false })
			Expect(err).ToNot(HaveOccurred())
			Expect(ri).ToNot(BeNil())
			Expect(rc).To(BeNil())
		})
	})

	Context("write rejection", func() {
		notSupported := func(err error) bool { return errors.As(err, new(errtypes.NotSupported)) }

		It("rejects CreateDir", func() {
			_, err := d.CreateDir(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID, OpaqueId: fix.spaceID}})
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects TouchFile", func() {
			_, err := d.TouchFile(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}}, false, "")
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects Delete", func() {
			_, err := d.Delete(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}})
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects Move", func() {
			ref := &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}}
			_, err := d.Move(fix.ctx, ref, ref)
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects SetLock", func() {
			_, err := d.SetLock(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}}, &provider.Lock{LockId: "x"})
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects AddGrant", func() {
			err := d.AddGrant(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}}, &provider.Grant{})
			Expect(err).To(Satisfy(notSupported))
		})
		It("rejects InitiateUpload", func() {
			_, err := d.InitiateUpload(fix.ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: fix.spaceID}}, 0, nil)
			Expect(err).To(Satisfy(notSupported))
		})
	})
})
