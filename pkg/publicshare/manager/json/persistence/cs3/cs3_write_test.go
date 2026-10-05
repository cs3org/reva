package cs3_test

import (
	"context"
	"testing"

	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/require"
)

// afterUploadStorage runs a hook once, in the window between an Upload
// completing and the caller's next call. It stands in for a second instance
// writing the same publicshares.json concurrently - the interleaving that
// makes a cached mtime sampled *after* an upload describe someone else's
// content rather than our own.
type afterUploadStorage struct {
	metadata.Storage
	hook func()
}

func (s *afterUploadStorage) Upload(ctx context.Context, req metadata.UploadRequest) (*metadata.UploadResponse, error) {
	res, err := s.Storage.Upload(ctx, req)
	if err == nil && s.hook != nil {
		hook := s.hook
		s.hook = nil
		hook()
	}
	return res, err
}

// newInstances returns two independent cs3 persistences over one shared
// publicshares.json - "a", whose uploads trigger a hook, and "b", standing in
// for another process writing the same file.
func newInstances(t *testing.T) (a persistence.Persistence, b persistence.Persistence, hooked *afterUploadStorage) {
	t.Helper()

	disk, err := metadata.NewDiskStorage(t.TempDir())
	require.NoError(t, err)

	hooked = &afterUploadStorage{Storage: disk}

	a = cs3.New(hooked)
	require.NoError(t, a.Init(context.Background()))

	b = cs3.New(disk)
	require.NoError(t, b.Init(context.Background()))

	return a, b, hooked
}

// TestWriteDoesNotAdoptAConcurrentWritersMtime pins the cache invariant that
// the cached mtime must always describe the cached *content*. Sampling the
// mtime from a separate Stat after the upload breaks that: a second instance
// writing in between makes this instance cache a foreign mtime next to its own
// content, and from then on every Stat compares equal, so the newer remote
// content is never fetched again.
func TestWriteDoesNotAdoptAConcurrentWritersMtime(t *testing.T) {
	ctx := context.Background()
	a, b, hooked := newInstances(t)

	_, err := a.Read(ctx)
	require.NoError(t, err)

	hooked.hook = func() {
		db, err := b.Read(ctx)
		require.NoError(t, err)
		db["from-b"] = map[string]interface{}{"share": "b", "password": ""}
		require.NoError(t, b.Write(ctx, db))
	}

	require.NoError(t, a.Write(ctx, persistence.PublicShares{
		"from-a": map[string]interface{}{"share": "a", "password": ""},
	}))

	got, err := a.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, got, "from-b", "a share written by another instance must not stay invisible behind a stale cache")
}

// TestWriteDoesNotDropAConcurrentWritersShare is the data-loss consequence of
// the same stale cache, via the read-modify-write cycle every manager write
// path performs: if the Read serving that cycle is stale, the share it never
// saw is silently overwritten - and the IfUnmodifiedSince guard cannot catch
// it, because the mtime it carries is the concurrent writer's own.
func TestWriteDoesNotDropAConcurrentWritersShare(t *testing.T) {
	ctx := context.Background()
	a, b, hooked := newInstances(t)

	_, err := a.Read(ctx)
	require.NoError(t, err)

	hooked.hook = func() {
		db, err := b.Read(ctx)
		require.NoError(t, err)
		db["from-b"] = map[string]interface{}{"share": "b", "password": ""}
		require.NoError(t, b.Write(ctx, db))
	}

	require.NoError(t, a.Write(ctx, persistence.PublicShares{
		"from-a": map[string]interface{}{"share": "a", "password": ""},
	}))

	db, err := a.Read(ctx)
	require.NoError(t, err)
	db["from-a2"] = map[string]interface{}{"share": "a2", "password": ""}
	require.NoError(t, a.Write(ctx, db))

	// Assert against a third instance so the assertion reads the file rather
	// than either writer's cache.
	fresh, err := b.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, fresh, "from-b", "a concurrent writer's share must survive a later read-modify-write")
	require.Contains(t, fresh, "from-a")
	require.Contains(t, fresh, "from-a2")
}
