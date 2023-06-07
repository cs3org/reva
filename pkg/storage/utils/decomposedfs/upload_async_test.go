package decomposedfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/events/stream"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	treemocks "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/store"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/tests/helpers"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Async file uploads", Ordered, func() {
	var (
		ref = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId: "u-s-e-r-id",
			},
			Path: "/foo",
		}

		rootRef = &provider.Reference{
			ResourceId: &provider.ResourceId{
				SpaceId:  "u-s-e-r-id",
				OpaqueId: "u-s-e-r-id",
			},
			Path: "/",
		}

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "u-s-e-r-id",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "username",
		}

		fileContent = []byte("0123456789")

		ctx = ruser.ContextSetUser(context.Background(), user)

		pub      chan interface{}
		con      chan interface{}
		uploadID string

		fs                   storage.FS
		o                    *options.Options
		lu                   *lookup.Lookup
		permissions          *mocks.PermissionsChecker
		cs3permissionsclient *mocks.CS3PermissionsClient
		permissionsSelector  pool.Selectable[cs3permissions.PermissionsAPIClient]
		bs                   *treemocks.Blobstore
	)

	BeforeEach(func() {
		// setup test
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		o, err = options.New(map[string]interface{}{
			"root":             tmpRoot,
			"asyncfileuploads": true,
		})
		Expect(err).ToNot(HaveOccurred())

		lu = lookup.New(metadata.XattrsBackend{}, o)
		permissions = &mocks.PermissionsChecker{}

		cs3permissionsclient = &mocks.CS3PermissionsClient{}
		pool.RemoveSelector("PermissionsSelector" + "any")
		permissionsSelector = pool.GetSelector[cs3permissions.PermissionsAPIClient](
			"PermissionsSelector",
			"any",
			func(cc *grpc.ClientConn) cs3permissions.PermissionsAPIClient {
				return cs3permissionsclient
			},
		)
		bs = &treemocks.Blobstore{}

		// create space uses CheckPermission endpoint
		cs3permissionsclient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&cs3permissions.CheckPermissionResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		}, nil).Times(1)

		// for this test we don't care about permissions
		permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
			Return(provider.ResourcePermissions{
				Stat:               true,
				GetQuota:           true,
				InitiateFileUpload: true,
				ListContainer:      true,
				ListFileVersions:   true,
			}, nil)

		// setup fs
		pub, con = make(chan interface{}), make(chan interface{})
		tree := tree.New(lu, bs, o, store.Create())
		fs, err = New(o, lu, NewPermissions(permissions, permissionsSelector), tree, stream.Chan{pub, con})
		Expect(err).ToNot(HaveOccurred())

		resp, err := fs.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{Owner: user, Type: "personal"})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Status.Code).To(Equal(v1beta11.Code_CODE_OK))
		resID, err := storagespace.ParseID(resp.StorageSpace.Id.OpaqueId)
		Expect(err).ToNot(HaveOccurred())
		ref.ResourceId = &resID

		bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("string"), mock.Anything).
			Return(nil).
			Run(func(args mock.Arguments) {
				data, err := os.ReadFile(args.Get(1).(string))

				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal(fileContent))
			})

		// start upload of a file
		uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(uploadIds)).To(Equal(2))
		Expect(uploadIds["simple"]).ToNot(BeEmpty())
		Expect(uploadIds["tus"]).ToNot(BeEmpty())

		uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

		_, err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(fileContent)), nil)
		Expect(err).ToNot(HaveOccurred())

		uploadID = uploadIds["simple"]

		// wait for bytes received event
		_, ok := (<-pub).(events.BytesReceived)
		Expect(ok).To(BeTrue())

		// blobstore not called yet
		bs.AssertNumberOfCalls(GinkgoT(), "Upload", 0)
	})

	AfterEach(func() {
		if o.Root != "" {
			os.RemoveAll(o.Root)
		}
		close(pub)
		close(con)
	})

	When("the uploaded file is new", func() {
		It("succeeds eventually", func() {
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
			_, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())

			// blobstore called now
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 1)

			// node ready
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item = resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(BeEmpty())

		})

		It("deletes node and bytes when instructed", func() {
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
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 0)

			// node gone
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(0))

			// bytes gone
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).ToNot(BeNil())
		})

		It("deletes node and keeps the bytes when instructed", func() {
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
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 0)

			// node gone
			resources, err = fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(0))

			// bytes are still here
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).To(BeNil())
		})
	})

	When("the uploaded file creates a new version", func() {
		JustBeforeEach(func() {
			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeContinue,
			}

			// wait for upload to be ready
			_, ok := (<-pub).(events.UploadReady)
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

			// wait for bytes received event
			_, ok = (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())

			// version already created
			revs, err = fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(1))

			// at this stage: blobstore called once for the original file
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 1)

		})

		It("succeeds eventually, creating a new version", func() {
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
			revs, err := fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(1))

			// blobstore now called twice - for original file and new version
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 2)

			// bytes are gone from upload path
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).ToNot(BeNil())
		})

		It("removes new version and restores old one when instructed", func() {
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
			revs, err := fs.ListRevisions(ctx, ref)
			Expect(err).To(BeNil())
			Expect(len(revs)).To(Equal(0))

			// bytes are removed from upload path
			_, err = os.Stat(filepath.Join(o.Root, "uploads", uploadID))
			Expect(err).ToNot(BeNil())

			// blobstore still called only once for the original file
			bs.AssertNumberOfCalls(GinkgoT(), "Upload", 1)
		})

	})
	When("Two uploads are happening in parallel", func() {
		var secondUploadID string

		JustBeforeEach(func() {
			// upload again
			uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(uploadIds)).To(Equal(2))
			Expect(uploadIds["simple"]).ToNot(BeEmpty())
			Expect(uploadIds["tus"]).ToNot(BeEmpty())

			uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

			_, err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(fileContent)), nil)
			Expect(err).ToNot(HaveOccurred())

			secondUploadID = uploadIds["simple"]

			// wait for bytes received event
			_, ok := (<-pub).(events.BytesReceived)
			Expect(ok).To(BeTrue())
		})

		It("doesn't remove processing status when first upload is finished", func() {
			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: uploadID,
				Outcome:  events.PPOutcomeContinue,
			}
			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeFalse())

			// check processing status
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal("processing"))
		})

		It("removes processing status when second upload is finished, even if first isn't", func() {
			// finish postprocessing
			con <- events.PostprocessingFinished{
				UploadID: secondUploadID,
				Outcome:  events.PPOutcomeContinue,
			}
			// wait for upload to be ready
			ev, ok := (<-pub).(events.UploadReady)
			Expect(ok).To(BeTrue())
			Expect(ev.Failed).To(BeFalse())

			// check processing status
			resources, err := fs.ListFolder(ctx, rootRef, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(resources)).To(Equal(1))

			item := resources[0]
			Expect(item.Path).To(Equal(ref.Path))
			Expect(utils.ReadPlainFromOpaque(item.Opaque, "status")).To(Equal(""))
		})
	})
})
