package decomposedfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/events/stream"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/tests/helpers"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Async file uploads", Ordered, func() {
	var (
		ref     *provider.Reference
		rootRef *provider.Reference
		fs      storage.FS
		user    *userpb.User
		ctx     context.Context

		o                    *options.Options
		lu                   *lookup.Lookup
		permissions          *mocks.PermissionsChecker
		cs3permissionsclient *mocks.CS3PermissionsClient
		bs                   *treemocks.Blobstore

		pub chan interface{}
		con chan interface{}

		fileContent = []byte("0123456789")
		uploadID    string
	)

	BeforeAll(func() {
		ref = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId: "u-s-e-r-id",
			},
			Path: "/foo",
		}

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "u-s-e-r-id",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "username",
		}

		rootRef = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId:  "u-s-e-r-id",
				OpaqueId: "u-s-e-r-id",
			},
			Path: "/",
		}

		ctx = ruser.ContextSetUser(context.Background(), user)
	})

	BeforeEach(func() {

		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		o, err = options.New(map[string]interface{}{
			"root": tmpRoot,
		})
		Expect(err).ToNot(HaveOccurred())

		lu = &lookup.Lookup{Options: o}
		permissions = &mocks.PermissionsChecker{}
		cs3permissionsclient = &mocks.CS3PermissionsClient{}
		bs = &treemocks.Blobstore{}

		cs3permissionsclient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&cs3permissions.CheckPermissionResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		}, nil).Times(1)
		permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
			Stat:     true,
			AddGrant: true,
		}, nil).Times(1)

		tree := tree.New(o.Root, true, true, lu, bs)
		fs, err = New(o, lu, permissions, tree, cs3permissionsclient)
		Expect(err).ToNot(HaveOccurred())

		pub, con = make(chan interface{}), make(chan interface{})
		fs.(*Decomposedfs).o.AsyncFileUploads = true
		fs.(*Decomposedfs).stream = stream.Chan{pub, con}
		go fs.(*Decomposedfs).Postprocessing(con)

		resp, err := fs.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{Owner: user, Type: "personal"})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Status.Code).To(Equal(v1beta11.Code_CODE_OK))
		resID, err := storagespace.ParseID(resp.StorageSpace.Id.OpaqueId)
		Expect(err).ToNot(HaveOccurred())
		ref.ResourceId = &resID

		permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
			Return(provider.ResourcePermissions{
				Stat:               true,
				GetQuota:           true,
				InitiateFileUpload: true,
				ListContainer:      true,
				ListFileVersions:   true,
			}, nil)

		bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("*os.File"), mock.Anything).
			Return(nil).
			Run(func(args mock.Arguments) {
				reader := args.Get(1).(io.Reader)
				data, err := io.ReadAll(reader)

				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal(fileContent))
			})

		uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(uploadIds)).To(Equal(2))
		Expect(uploadIds["simple"]).ToNot(BeEmpty())
		Expect(uploadIds["tus"]).ToNot(BeEmpty())

		uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

		_, err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(fileContent)), nil)
		Expect(err).ToNot(HaveOccurred())

		uploadID = uploadIds["simple"]
	})

	AfterEach(func() {
		root := o.Root
		if root != "" {
			os.RemoveAll(root)
		}
	})

	AfterAll(func() {
		time.Sleep(time.Second)
		close(pub)
		close(con)
	})

	When("the uploaded file is new", func() {
		It("succeeds eventually", func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// blobstore not called yet
			bs.AssertNotCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node is created
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal("processing"))

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeContinue,
			}

			// wait for upload to be ready
			_, ok = (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())

			// blobstore called now
			bs.AssertCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node ready
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item = resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(BeEmpty())

		})

		It("deletes node and bytes when instructed", func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// blobstore not called yet
			bs.AssertNotCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node is created
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal("processing"))

			// bytes are in dedicated path
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).To(BeNil())

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeDelete,
			}

			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeTrue())

			// blobstore still not called now
			bs.AssertNotCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node gone
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(0))

			// bytes gone
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).ToNot(BeNil())
		})

		It("deletes node and keeps the bytes when instructed", func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// blobstore not called yet
			bs.AssertNotCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node is created
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal("processing"))

			// bytes are in dedicated path
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).To(BeNil())

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeAbort,
			}

			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeTrue())

			// blobstore still not called now
			bs.AssertNotCalled(GinkgoT(), "Upload", mock.Anything, mock.Anything, mock.Anything)

			// node gone
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(0))

			// bytes are still here
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).To(BeNil())
		})
	})

	When("the uploaded file is new", func() {
		JustBeforeEach(func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeContinue,
			}

			// wait for upload to be ready
			_, ok = (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())

			// make sure there is no version yet
			revs, err := fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(0))

			// upload again
			uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(uploadIds)).To(Equal(2))
			Expect(uploadIds["simple"]).ToNot(BeEmpty())
			Expect(uploadIds["tus"]).ToNot(BeEmpty())

			uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

			_, err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(fileContent)), nil)
			Expect(err).ToNot(HaveOccurred())

			uploadID = uploadIds["simple"]
		})

		It("succeeds eventually, creating a new version", func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// version already created
			revs, err := fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(1))

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeContinue,
			}
			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeFalse())

			// version still existing
			revs, err = fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(1))
		})

		It("removes new version and restores old one when instructed", func() {
			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// version already created
			revs, err := fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(1))

			// node exists and is processing
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))
			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal("processing"))

			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeDelete,
			}
			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeTrue())

			// node still exists as old version is restored
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))
			item = resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(BeEmpty())

			// version gone now
			revs, err = fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(0))
		})

	})
})
