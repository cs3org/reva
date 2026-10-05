package cs3_test

import (
	"context"
	"testing"

	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/require"
)

// TestReadArmsTheWritePrecondition pins the guard that stops two instances from
// overwriting each other: Write sends the mtime Read observed as
// IfUnmodifiedSince, so a write that lands in between has to be rejected rather
// than silently dropped. Both backends skip the check entirely when that mtime
// is the zero time, so a Read that fails to record a usable mtime disarms the
// guard without failing anything - hence asserting on the rejection itself.
func TestReadArmsTheWritePrecondition(t *testing.T) {
	ctx := context.Background()

	disk, err := metadata.NewDiskStorage(t.TempDir())
	require.NoError(t, err)

	a := cs3.New(disk)
	require.NoError(t, a.Init(ctx))
	b := cs3.New(disk)
	require.NoError(t, b.Init(ctx))

	require.NoError(t, a.Write(ctx, persistence.PublicShares{
		"from-a": map[string]interface{}{"share": "a", "password": ""},
	}))

	// a reads, so it now holds the mtime its next write will be checked against.
	_, err = a.Read(ctx)
	require.NoError(t, err)

	// b writes in between, moving the file past the mtime a is holding.
	db, err := b.Read(ctx)
	require.NoError(t, err)
	db = persistence.Copy(db)
	db["from-b"] = map[string]interface{}{"share": "b", "password": ""}
	require.NoError(t, b.Write(ctx, db))

	err = a.Write(ctx, persistence.PublicShares{
		"from-a": map[string]interface{}{"share": "a", "password": ""},
	})
	require.Error(t, err, "a write racing a concurrent one must be rejected, not silently overwrite it")
}

// TestReadOnMissingFileDoesNotPanic covers the very first read of an instance
// whose publicshares.json has not been created yet: there is nothing to
// download, which has to surface as an empty database rather than a failure
// deeper in the storage layer.
func TestReadOnMissingFileDoesNotPanic(t *testing.T) {
	ctx := context.Background()

	disk, err := metadata.NewDiskStorage(t.TempDir())
	require.NoError(t, err)

	p := cs3.New(disk)
	require.NoError(t, p.Init(ctx))

	db, err := p.Read(ctx)
	require.NoError(t, err)
	require.Empty(t, db)
}
