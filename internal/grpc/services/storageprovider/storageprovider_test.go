package storageprovider

import (
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
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
