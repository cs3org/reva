package grpc_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	grpcMetadata "google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/appctx"
	"github.com/owncloud/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/owncloud/reva/v2/pkg/share/manager/jsoncs3/receivedsharecache"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	jwt "github.com/owncloud/reva/v2/pkg/token/manager/jwt"
	"github.com/rs/zerolog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("receivedsharecache concurrent CS3 writes", func() {
	var (
		revads    map[string]*Revad
		ctx       context.Context
		spaceRoot *provider.ResourceId

		csUser = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}

		csUserID  = "user"
		csSpaceID = "spaceid"
	)

	BeforeEach(func() {
		var err error
		zl := zerolog.New(os.Stdout).Level(zerolog.DebugLevel)
		ctx = appctx.WithLogger(context.Background(), &zl)

		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		sc, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, csUser, sc)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = grpcMetadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, csUser)

		revads, err = startRevads([]RevadConfig{
			{Name: "storage", Config: "storageprovider-ocis-with-dataprovider.toml"},
			{Name: "permissions", Config: "permissions-ocis-ci.toml"},
		}, nil)
		Expect(err).ToNot(HaveOccurred())

		spacesClient, err := pool.GetSpacesProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
		res, err := spacesClient.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
			Owner: csUser,
			Type:  "metadata",
			Name:  "Metadata",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code.String()).To(Equal("CODE_OK"))
		spaceRoot = res.StorageSpace.Root

		// decomposedfs CreateContainer requires parent to exist; pre-create /users.
		setup := metadata.NewCS3("", revads["storage"].GrpcAddress)
		setup.SpaceRoot = spaceRoot
		Expect(setup.MakeDirIfNotExist(ctx, "/users")).To(Succeed())
	})

	AfterEach(func() {
		for _, r := range revads {
			r.Cleanup(CurrentSpecReport().Failed()) //nolint:errcheck
		}
		pool.RemoveSelector("StorageProviderSelector" + revads["storage"].GrpcAddress)
	})

	It("preserves all shares when 2 replicas write concurrently (OCISDEV-855)", func() {
		newCS3 := func() *metadata.CS3 {
			cs3 := metadata.NewCS3("", revads["storage"].GrpcAddress)
			cs3.SpaceRoot = spaceRoot
			return cs3
		}

		const numShares = 15
		replicas := [2]receivedsharecache.Cache{
			receivedsharecache.New(newCS3(), 0),
			receivedsharecache.New(newCS3(), 0),
		}

		errs := make([]error, numShares)
		var wg sync.WaitGroup
		for i := 0; i < numShares; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				rs := &collaboration.ReceivedShare{
					Share: &collaboration.Share{
						Id: &collaboration.ShareId{OpaqueId: fmt.Sprintf("share-%d", idx)},
					},
					State: collaboration.ShareState_SHARE_STATE_PENDING,
				}
				errs[idx] = replicas[idx%2].Add(ctx, csUserID, csSpaceID, rs)
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			Expect(err).ToNot(HaveOccurred(), "Add failed for share-%d", i)
		}

		fresh := receivedsharecache.New(newCS3(), 0)
		spaces, err := fresh.List(ctx, csUserID)
		Expect(err).ToNot(HaveOccurred())
		Expect(spaces[csSpaceID]).ToNot(BeNil())
		for i := 0; i < numShares; i++ {
			Expect(spaces[csSpaceID].States).To(HaveKey(fmt.Sprintf("share-%d", i)))
		}
	})

	It("both replicas recover when writes are forced simultaneous (OCISDEV-855)", func() {
		newCS3 := func() *metadata.CS3 {
			cs3 := metadata.NewCS3("", revads["storage"].GrpcAddress)
			cs3.SpaceRoot = spaceRoot
			return cs3
		}

		bs := newBarrierStorageCS3(newCS3(), 2)
		replicas := [2]receivedsharecache.Cache{
			receivedsharecache.New(bs, 0),
			receivedsharecache.New(bs, 0),
		}

		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				rs := &collaboration.ReceivedShare{
					Share: &collaboration.Share{
						Id: &collaboration.ShareId{OpaqueId: fmt.Sprintf("share-%d", idx)},
					},
					State: collaboration.ShareState_SHARE_STATE_PENDING,
				}
				errs[idx] = replicas[idx].Add(ctx, csUserID, csSpaceID, rs)
			}(i)
		}
		wg.Wait()
		Expect(errs[0]).ToNot(HaveOccurred())
		Expect(errs[1]).ToNot(HaveOccurred())

		fresh := receivedsharecache.New(newCS3(), 0)
		spaces, err := fresh.List(ctx, csUserID)
		Expect(err).ToNot(HaveOccurred())
		Expect(spaces[csSpaceID]).ToNot(BeNil())
		Expect(spaces[csSpaceID].States).To(HaveKey("share-0"))
		Expect(spaces[csSpaceID].States).To(HaveKey("share-1"))
	})
})

// barrierStorageCS3 holds Upload calls until n goroutines have arrived, then
// releases all simultaneously — forcing the concurrent-write race deterministically.
type barrierStorageCS3 struct {
	metadata.Storage
	arrived   int32
	n         int32
	ready     chan struct{}
	closeOnce sync.Once
}

func newBarrierStorageCS3(s metadata.Storage, n int) *barrierStorageCS3 {
	return &barrierStorageCS3{Storage: s, n: int32(n), ready: make(chan struct{})}
}

func (b *barrierStorageCS3) Upload(ctx context.Context, req metadata.UploadRequest) (*metadata.UploadResponse, error) {
	if atomic.AddInt32(&b.arrived, 1) >= b.n {
		b.closeOnce.Do(func() { close(b.ready) })
	}
	<-b.ready
	return b.Storage.Upload(ctx, req)
}
