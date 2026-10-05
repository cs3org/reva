package cs3_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/require"
)

// TestReadDoesNotCopyTheDatabase pins Read's cost to the size of the request
// rather than the size of publicshares.json. Read serves every
// ListPublicShares / GetPublicShareByToken call, so duplicating the whole share
// database on each one is a per-request cost that grows with the instance -
// exactly the instances already suffering from read-path contention. Callers
// that mutate what Read returns are the ones that have to copy.
func TestReadDoesNotCopyTheDatabase(t *testing.T) {
	ctx := context.Background()

	disk, err := metadata.NewDiskStorage(t.TempDir())
	require.NoError(t, err)

	p := cs3.New(disk)
	require.NoError(t, p.Init(ctx))

	const shares = 1000
	db := persistence.PublicShares{}
	for i := range shares {
		db[fmt.Sprintf("share-%d", i)] = map[string]interface{}{
			"share":    fmt.Sprintf("payload-%d", i),
			"password": "",
		}
	}
	require.NoError(t, p.Write(ctx, db))

	// Warm the cache so the measured calls neither download nor unmarshal.
	_, err = p.Read(ctx)
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(20, func() {
		if _, err := p.Read(ctx); err != nil {
			t.Fatal(err)
		}
	})

	// Copying the database allocates a map per share, so it cannot come in
	// under this bound; a Read that hands out the cache stays far below it.
	require.Less(t, allocs, float64(shares/5),
		"Read allocated %.0f times serving %d shares, which means it is copying the database", allocs, shares)
}
