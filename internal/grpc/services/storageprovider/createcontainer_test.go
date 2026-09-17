package storageprovider

import (
	"context"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// fakeCreateDirFS returns canned results for CreateDir.
type fakeCreateDirFS struct {
	noopFS
	md  *provider.ResourceInfo
	err error
}

func (f *fakeCreateDirFS) CreateDir(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.md, nil
}

func TestCreateContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		md         *provider.ResourceInfo
		err        error
		wantCode   rpc.Code
		wantFileID string
		wantEtag   string
	}{
		{
			name: "driver returns the new dir info",
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{OpaqueId: "inode-42"},
				Etag: `"abc"`,
				Path: "/newdir",
			},
			wantCode: rpc.Code_CODE_OK,
			wantFileID: spaces.EncodeToStringifiedResourceID(&provider.ResourceId{
				StorageId: "test-mount-id",
				SpaceId:   spaces.EncodeSpaceID("/home"),
				OpaqueId:  "inode-42",
			}),
			wantEtag: `"abc"`,
		},
		{
			name:     "driver returns no info",
			wantCode: rpc.Code_CODE_OK,
		},
		{
			name:     "dir already exists",
			err:      errtypes.AlreadyExists("newdir"),
			wantCode: rpc.Code_CODE_ALREADY_EXISTS,
		},
		{
			name:     "parent does not exist",
			err:      errtypes.NotFound("newdir"),
			wantCode: rpc.Code_CODE_NOT_FOUND,
		},
		{
			name:     "permission denied",
			err:      errtypes.PermissionDenied("newdir"),
			wantCode: rpc.Code_CODE_PERMISSION_DENIED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &service{
				conf:      &config{},
				storage:   &fakeCreateDirFS{md: tt.md, err: tt.err},
				mountPath: "/home",
				mountID:   "test-mount-id",
			}

			res, err := svc.CreateContainer(context.Background(), &provider.CreateContainerRequest{
				Ref: &provider.Reference{Path: "/home/newdir"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status.Code != tt.wantCode {
				t.Fatalf("expected status %v, got %v (%s)", tt.wantCode, res.Status.Code, res.Status.Message)
			}

			var gotFileID, gotEtag string
			if res.Opaque != nil {
				if e, ok := res.Opaque.Map["fileid"]; ok {
					gotFileID = string(e.Value)
				}
				if e, ok := res.Opaque.Map["etag"]; ok {
					gotEtag = string(e.Value)
				}
			}
			if gotFileID != tt.wantFileID {
				t.Errorf("expected fileid %q, got %q", tt.wantFileID, gotFileID)
			}
			if gotEtag != tt.wantEtag {
				t.Errorf("expected etag %q, got %q", tt.wantEtag, gotEtag)
			}
		})
	}
}
