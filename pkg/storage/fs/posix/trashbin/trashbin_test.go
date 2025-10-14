package trashbin_test

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/prefixes"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/lookup"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/options"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/trashbin"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata/mocks"
)

func TestTrashbin_RestoreRecycleItem(t *testing.T) {
	backend := mocks.NewBackend(t)
	tb, err := trashbin.New(
		nil,
		nil,
		lookup.New(backend, nil, &options.Options{}, nil),
		nil,
	)
	assert.NoError(t, err)

	itemReference := &provider.Reference{
		ResourceId: &provider.ResourceId{
			OpaqueId: "123",
			SpaceId:  "123",
		},
		Path: "path/with/basename",
	}

	t.Run("restoring resources updates crucial extended attributes", func(t *testing.T) {
		// first call to get the id
		backend.EXPECT().IdentifyPath(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, string) (string, string, string, time.Time, error) {
			return "", "id", "", time.Time{}, nil
		}).Once()

		// first call to get the parentID
		backend.EXPECT().IdentifyPath(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, string) (string, string, string, time.Time, error) {
			return "", "parentID", "", time.Time{}, nil
		}).Once()

		backend.EXPECT().SetMultiple(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ metadata.MetadataNode, m map[string][]byte, _ bool) error {
			nameAttr, ok := m[prefixes.NameAttr]
			assert.True(t, ok)
			assert.Equal(t, nameAttr, []byte("basename"))

			parentIDAttr, ok := m[prefixes.ParentidAttr]
			assert.True(t, ok)
			assert.Equal(t, parentIDAttr, []byte("parentID"))

			return nil
		}).Once()

		_, err = tb.RestoreRecycleItem(t.Context(), "", "", "", itemReference)
		assert.Error(t, err)

		// the main test case is to check if the extended attributes are updated,
		// having an error at that stage is ok!
		var errLink *os.LinkError
		assert.ErrorAs(t, err, &errLink)
		assert.Equal(t, errLink.Err, syscall.ENOENT)
		assert.Equal(t, errLink.New, itemReference.Path)

		backend.EXPECT().IdentifyPath(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, string) (string, string, string, time.Time, error) {
			return "", "id", "", time.Time{}, nil
		}).Once()

		backend.EXPECT().IdentifyPath(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, string) (string, string, string, time.Time, error) {
			return "", "parentID", "", time.Time{}, nil
		}).Once()

		backend.EXPECT().SetMultiple(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ metadata.MetadataNode, m map[string][]byte, _ bool) error {
			nameAttr, ok := m[prefixes.NameAttr]
			assert.True(t, ok)
			assert.Equal(t, nameAttr, []byte("BASENAME (1)"))

			return nil
		}).Once()

		itemReference.Path = "path/with/BASENAME (1)"
		_, err = tb.RestoreRecycleItem(t.Context(), "", "", "", itemReference)
		assert.Error(t, err)
	})
}
