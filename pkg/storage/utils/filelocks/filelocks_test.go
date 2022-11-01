package filelocks_test

import (
	"os"
	"sync"
	"testing"

	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/stretchr/testify/assert"
)

func TestAcquireWriteLock(t *testing.T) {
	file, fin, _ := fileFactory()
	defer fin()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			l, err := filelocks.AcquireWriteLock(file)
			assert.Nil(t, err)
			assert.NotNil(t, l)
			assert.Equal(t, true, l.Locked())
			assert.Equal(t, false, l.RLocked())

			err = filelocks.ReleaseLock(l)
			assert.Nil(t, err)

			wg.Done()
		}()
	}

	wg.Wait()
}

func TestAcquireReadLock(t *testing.T) {
	file, fin, _ := fileFactory()
	defer fin()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			l, err := filelocks.AcquireReadLock(file)
			assert.Nil(t, err)
			assert.NotNil(t, l)
			assert.Equal(t, false, l.Locked())
			assert.Equal(t, true, l.RLocked())

			err = filelocks.ReleaseLock(l)
			assert.Nil(t, err)

			wg.Done()
		}()
	}

	wg.Wait()
}

func TestReleaseLock(t *testing.T) {
	file, fin, _ := fileFactory()
	defer fin()

	l1, err := filelocks.AcquireWriteLock(file)
	assert.Equal(t, true, l1.Locked())

	err = filelocks.ReleaseLock(l1)
	assert.Nil(t, err)
	assert.Equal(t, false, l1.Locked())
}

// test unexported

func TestAcquireLock(t *testing.T) {
	l1, err := filelocks.AcquireLock("", false)
	assert.Nil(t, l1)
	assert.Equal(t, err, filelocks.ErrPathEmpty)

	file, fin, _ := fileFactory()
	defer fin()

	l2, err := filelocks.AcquireLock(file, false)
	assert.NotNil(t, l2)
	assert.Nil(t, err)

	l3, err := filelocks.AcquireLock(file, false)
	assert.Nil(t, l3)
	assert.Equal(t, err, filelocks.ErrAcquireLockFailed)
}

// utils

func fileFactory() (string, func(), error) {
	fu := func() {}
	tmpFile, err := os.CreateTemp(os.TempDir(), "flock")
	if err != nil {
		return "", fu, err
	}

	fu = func() {
		_ = os.Remove(tmpFile.Name())
	}

	err = tmpFile.Close()
	if err != nil {
		return "", fu, err
	}

	return tmpFile.Name(), fu, err
}
