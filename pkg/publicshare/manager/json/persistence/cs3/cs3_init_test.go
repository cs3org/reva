package cs3_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/require"
)

// blockingStatStorage wraps a real metadata.Storage and parks Stat until the
// test releases it, reporting on entered that it has been reached. That gives a
// test two exact signals - "Read is now inside Stat, holding whatever lock it
// took" and "Read may proceed" - instead of a sleep long enough to hope the
// same thing has happened by now.
type blockingStatStorage struct {
	metadata.Storage

	enteredOnce sync.Once
	entered     chan struct{}
	release     chan struct{}
}

func (s *blockingStatStorage) Stat(ctx context.Context, path string) (*provider.ResourceInfo, error) {
	s.enteredOnce.Do(func() { close(s.entered) })
	<-s.release
	return s.Storage.Stat(ctx, path)
}

// TestInitDoesNotQueueBehindRead guards against a regression where a warm
// Init (i.e. one called after the persistence layer already reports itself
// initialized) shares cs3's mu with Read, and so queues behind whatever Read
// is doing - even though a warm Init only needs to check a bool. That would
// defeat the point of removing the manager's own equivalent lock (see
// json.go's init): moving that lock out of the way only helps if calls
// coming in right behind it don't just pile up on this one instead.
func TestInitDoesNotQueueBehindRead(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "cs3-init-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	disk, err := metadata.NewDiskStorage(tmpdir)
	require.NoError(t, err)

	blocking := &blockingStatStorage{
		Storage: disk,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	p := cs3.New(blocking)
	require.NoError(t, p.Init(context.Background()))

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = p.Read(context.Background())
	}()

	// Read is inside its Stat from here on, so it is holding mu for as long as
	// we withhold release.
	<-blocking.entered

	var initErr error
	initDone := make(chan struct{})
	go func() {
		defer close(initDone)
		initErr = p.Init(context.Background())
	}()

	// A warm Init touches nothing but an atomic bool, so it has to return while
	// Read is still parked. The timeout is only here to break the deadlock a
	// regression would cause - it is never waited on when the test passes, so
	// it can be generous without weakening the assertion.
	select {
	case <-initDone:
		require.NoError(t, initErr)
	case <-time.After(10 * time.Second):
		close(blocking.release)
		<-readDone
		t.Fatal("warm Init blocked on a Read that is parked inside Stat, so it is contending for Read's lock")
	}

	close(blocking.release)
	<-readDone
}
