package storageprovider

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
)

// singleFileRootMountRemap is shared by InitiateFileDownload (simple protocol)
// and InitiateFileUpload when ExposeDataServer is enabled.
func TestSingleFileRootMountRemapKeepsSharePathForMountedRootFile(t *testing.T) {
	t.Parallel()

	got := singleFileRootMountRemap("/", &provider.ResourceInfo{
		Type: provider.ResourceType_RESOURCE_TYPE_FILE,
		Path: "/185e771c-7c8c-422d-a080-d1c6bdf51ea1",
	})

	if got != "/185e771c-7c8c-422d-a080-d1c6bdf51ea1" {
		t.Fatalf("expected remapped storage path for mount-root file, got %q", got)
	}
}

func TestSingleFileRootMountRemapLeavesMountedRootFolderAlone(t *testing.T) {
	t.Parallel()

	got := singleFileRootMountRemap("/", &provider.ResourceInfo{
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path: "/185e771c-7c8c-422d-a080-d1c6bdf51ea1",
	})

	if got != "/" {
		t.Fatalf("expected root path to stay unchanged for folders, got %q", got)
	}
}

func TestSingleFileRootMountRemapLeavesNestedPathsAlone(t *testing.T) {
	t.Parallel()

	got := singleFileRootMountRemap("/nested/file.txt", &provider.ResourceInfo{
		Type: provider.ResourceType_RESOURCE_TYPE_FILE,
		Path: "/185e771c-7c8c-422d-a080-d1c6bdf51ea1",
	})

	if got != "/nested/file.txt" {
		t.Fatalf("expected nested path to stay unchanged, got %q", got)
	}
}

// getFS rebases space_depth on the paths the driver is called with: the mount
// path is trimmed off before a driver sees a reference, so a driver must not be
// handed a depth counted from this provider's namespace root.
func TestGetFSInjectsSpaceDepthRelativeToTheMountPath(t *testing.T) {
	tests := []struct {
		name       string
		mountPath  string
		spaceDepth int
		want       int
	}{
		// Roots at /winspaces/c/<project> are two levels into the driver's paths.
		{name: "mounted one level deep", mountPath: "/winspaces", spaceDepth: 3, want: 2},
		{name: "mounted at the root", mountPath: "/", spaceDepth: 2, want: 2},
		{name: "mounted several levels deep", mountPath: "/eos/project", spaceDepth: 4, want: 2},
		// A provider whose whole mount is a single space has no space root in the
		// driver's paths at all.
		{name: "single space", mountPath: "/winspaces", spaceDepth: 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := "test-space-depth-" + tt.name
			var got map[string]any
			registry.Register(driver, func(ctx context.Context, m map[string]any) (storage.FS, error) {
				got = m
				return noopFS{}, nil
			})
			t.Cleanup(func() { delete(registry.NewFuncs, driver) })

			c := &config{
				Driver:     driver,
				MountPath:  tt.mountPath,
				SpaceDepth: tt.spaceDepth,
				Drivers:    map[string]map[string]any{driver: {"some_option": "kept"}},
			}
			if _, err := getFS(context.Background(), c); err != nil {
				t.Fatalf("getFS: %v", err)
			}

			if got["space_depth"] != tt.want {
				t.Errorf("space_depth = %v, want %d", got["space_depth"], tt.want)
			}
			if got["some_option"] != "kept" {
				t.Errorf("the driver's own config was dropped: %v", got)
			}
			if _, ok := c.Drivers[driver]["space_depth"]; ok {
				t.Error("the parsed configuration was mutated")
			}
		})
	}
}

// restoreFS records the force of RestoreRecycleItem, and returns err.
type restoreFS struct {
	noopFS
	force bool
	err   error
}

func (f *restoreFS) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference, force bool) error {
	f.force = force
	return f.err
}

func TestRestoreRecycleItemReturnsTheCollisions(t *testing.T) {
	collisions := storage.RecycleCollisions{{Key: "20261123/docs/report.txt", Path: "docs/report.txt"}}
	fs := &restoreFS{err: collisions}
	s := &service{storage: fs, mountPath: "/"}

	res, err := s.RestoreRecycleItem(context.Background(), &provider.RestoreRecycleItemRequest{
		Ref: &provider.Reference{Path: "/proj"},
		Key: "20261123/docs",
	})
	if err != nil {
		t.Fatal(err)
	}

	if fs.force {
		t.Error("got force, want no force")
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_ALREADY_EXISTS {
		t.Errorf("got status %v, want %v", res.GetStatus().GetCode(), rpc.Code_CODE_ALREADY_EXISTS)
	}
	var got storage.RecycleCollisions
	if err := json.Unmarshal(res.GetOpaque().GetMap()[storage.RecycleRestoreCollisions].GetValue(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, collisions) {
		t.Errorf("got collisions %v, want %v", got, collisions)
	}
}

func TestRestoreRecycleItemPassesForce(t *testing.T) {
	fs := &restoreFS{}
	s := &service{storage: fs, mountPath: "/"}

	res, err := s.RestoreRecycleItem(context.Background(), &provider.RestoreRecycleItemRequest{
		Ref: &provider.Reference{Path: "/proj"},
		Key: "1a2b3c",
		Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{
			storage.RecycleRestoreForce: {Decoder: "plain", Value: []byte("true")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !fs.force {
		t.Error("got no force, want force")
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		t.Errorf("got status %v, want %v", res.GetStatus().GetCode(), rpc.Code_CODE_OK)
	}
}
