package storageprovider

import (
	"context"
	"io"
	"net/url"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// noopFS implements storage.FS with stubs so tests can embed it and override
// only the methods the code under test calls.
type noopFS struct{}

func (noopFS) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("noop")
}
func (noopFS) CreateHome(ctx context.Context) error { return errtypes.NotSupported("noop") }
func (noopFS) CreateDir(ctx context.Context, ref *provider.Reference) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) Delete(ctx context.Context, ref *provider.Reference) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) Download(ctx context.Context, ref *provider.Reference, ranges []storage.Range) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) EmptyRecycle(ctx context.Context) error { return errtypes.NotSupported("noop") }
func (noopFS) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return "", errtypes.NotSupported("noop")
}
func (noopFS) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) GetQuota(ctx context.Context, ref *provider.Reference) (uint64, uint64, error) {
	return 0, 0, errtypes.NotSupported("noop")
}
func (noopFS) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) Shutdown(ctx context.Context) error { return nil }
func (noopFS) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("noop")
}
func (noopFS) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("noop")
}
func (noopFS) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("noop")
}

var _ storage.FS = (*noopFS)(nil)

// captureRootMountFS returns fixed metadata for mount-root "/" and records the
// reference passed to InitiateUpload.
type captureRootMountFS struct {
	noopFS
	rootMD        *provider.ResourceInfo
	lastUploadRef *provider.Reference
}

func (f *captureRootMountFS) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	if ref.GetPath() == "/" && f.rootMD != nil {
		return f.rootMD, nil
	}
	return nil, errtypes.NotFound("unexpected ref in test fake")
}

func (f *captureRootMountFS) InitiateUpload(ctx context.Context, ref *provider.Reference, _ int64, _ map[string]string) (map[string]string, error) {
	cp := &provider.Reference{Path: ref.GetPath()}
	if ref.ResourceId != nil {
		cp.ResourceId = ref.ResourceId
	}
	f.lastUploadRef = cp
	return map[string]string{"simple": "test-upload-token"}, nil
}

func (f *captureRootMountFS) Shutdown(ctx context.Context) error { return nil }

func newServiceForInitiateUploadTest(t *testing.T, fs storage.FS, expose bool) *service {
	t.Helper()
	u, err := url.Parse("http://localhost/data")
	if err != nil {
		t.Fatal(err)
	}
	xs, err := parseXSTypes(map[string]uint32{"md5": 100})
	if err != nil {
		t.Fatal(err)
	}
	return &service{
		conf:          &config{ExposeDataServer: expose},
		storage:       fs,
		mountPath:     "/",
		mountID:       "test-mount-id",
		dataServerURL: u,
		availableXS:   xs,
	}
}

func TestInitiateFileUploadRemapsRootPathForSingleFileWithExposeDataServer(t *testing.T) {
	t.Parallel()

	const canonicalPath = "/185e771c-7c8c-422d-a080-d1c6bdf51ea1"
	fs := &captureRootMountFS{
		rootMD: &provider.ResourceInfo{
			Type: provider.ResourceType_RESOURCE_TYPE_FILE,
			Path: canonicalPath,
		},
	}
	svc := newServiceForInitiateUploadTest(t, fs, true)
	ctx := context.Background()

	res, err := svc.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("expected OK, got %v %s", res.Status.Code, res.Status.Message)
	}
	if fs.lastUploadRef == nil {
		t.Fatal("expected InitiateUpload to be called")
	}
	if got := fs.lastUploadRef.GetPath(); got != canonicalPath {
		t.Fatalf("InitiateUpload ref path: want %q, got %q", canonicalPath, got)
	}
}

func TestInitiateFileUploadRootRejectedWhenExposeDataServerOff(t *testing.T) {
	t.Parallel()

	fs := &captureRootMountFS{
		rootMD: &provider.ResourceInfo{
			Type: provider.ResourceType_RESOURCE_TYPE_FILE,
			Path: "/185e771c-7c8c-422d-a080-d1c6bdf51ea1",
		},
	}
	svc := newServiceForInitiateUploadTest(t, fs, false)
	ctx := context.Background()

	res, err := svc.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status.Code == rpc.Code_CODE_OK {
		t.Fatal("expected non-OK without ExposeDataServer remap")
	}
	if fs.lastUploadRef != nil {
		t.Fatal("InitiateUpload must not run when mount path guard rejects")
	}
}

func TestInitiateFileUploadRootFolderStillRejectedWithExposeDataServer(t *testing.T) {
	t.Parallel()

	fs := &captureRootMountFS{
		rootMD: &provider.ResourceInfo{
			Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			Path: "/some-space",
		},
	}
	svc := newServiceForInitiateUploadTest(t, fs, true)
	ctx := context.Background()

	res, err := svc.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status.Code == rpc.Code_CODE_OK {
		t.Fatal("expected non-OK for folder at mount root")
	}
	if fs.lastUploadRef != nil {
		t.Fatal("InitiateUpload must not run for folder mount root")
	}
}

func TestInitiateFileUploadRootGetMDErrorReportsStorageFailure(t *testing.T) {
	t.Parallel()

	fs := &captureRootMountFS{rootMD: nil}
	svc := newServiceForInitiateUploadTest(t, fs, true)
	ctx := context.Background()

	res, err := svc.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status.Code == rpc.Code_CODE_OK {
		t.Fatal("expected non-OK when GetMD fails at mount root")
	}
	if res.Status.Message != "error resolving mount-root resource for upload" {
		t.Fatalf("expected storage-resolution error message, got %q", res.Status.Message)
	}
	if fs.lastUploadRef != nil {
		t.Fatal("InitiateUpload must not run when GetMD fails")
	}
}
