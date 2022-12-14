package decomposedfs

import (
	"bytes"
	"context"
	"io"
	"os"

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

var _ = Describe("Async file uploads", func() {
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
	)

	BeforeEach(func() {
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
	})

	AfterEach(func() {
		root := o.Root
		if root != "" {
			os.RemoveAll(root)
		}
	})

	BeforeEach(func() {
		cs3permissionsclient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&cs3permissions.CheckPermissionResponse{
			Status: &v1beta11.Status{Code: v1beta11.Code_CODE_OK},
		}, nil)
		permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
			Stat:     true,
			AddGrant: true,
		}, nil).Times(1)
		var err error
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
	})

	Context("With async uploads", func() {
		BeforeEach(func() {
			permissions.On("AssemblePermissions", mock.Anything, mock.Anything).
				Return(provider.ResourcePermissions{
					Stat:               true,
					GetQuota:           true,
					InitiateFileUpload: true,
					ListContainer:      true,
				}, nil)
		})

		When("the user uploads a non zero byte file", func() {
			FIt("succeeds", func() {
				var (
					fileContent = []byte("0123456789")
				)

				bs.On("Upload", mock.AnythingOfType("*node.Node"), mock.AnythingOfType("*os.File"), mock.Anything).
					Return(nil).
					Run(func(args mock.Arguments) {
						reader := args.Get(1).(io.Reader)
						data, err := io.ReadAll(reader)

						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal([]byte("0123456789")))
					})

				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())

				uploadRef := &provider.Reference{Path: "/" + uploadIds["simple"]}

				_, err = fs.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader(fileContent)), nil)
				Expect(err).ToNot(HaveOccurred())

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
					UploadID: uploadIds["simple"],
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
		})
	})

})
