// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package filelocks_test

import (
	"sync"
	"testing"

	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"
)

func TestAcquireWriteLock(t *testing.T) {
	file, fin, _ := filelocks.FileFactory()
	defer fin()

	filelocks.SetMaxLockCycles(90)
	filelocks.SetLockCycleDurationFactor(3)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			l, err := filelocks.AcquireWriteLock(file)
			assert.Nil(t, err)
			require.NotNil(t, l)

			defer func() {
				err = filelocks.ReleaseLock(l)
				assert.Nil(t, err)
			}()

			assert.Equal(t, true, l.Locked())
			assert.Equal(t, false, l.RLocked())
		}()
	}

	wg.Wait()
}

func TestAcquireReadLock(t *testing.T) {
	file, fin, _ := filelocks.FileFactory()
	defer fin()

	filelocks.SetMaxLockCycles(90)
	filelocks.SetLockCycleDurationFactor(3)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			l, err := filelocks.AcquireReadLock(file)
			assert.Nil(t, err)
			require.NotNil(t, l)

			defer func() {
				err = filelocks.ReleaseLock(l)
				assert.Nil(t, err)
			}()

			assert.Equal(t, false, l.Locked())
			assert.Equal(t, true, l.RLocked())

		}()
	}

	wg.Wait()
}

/* This negative test is flaky as 8000 goroutines are not enough to trigger this in ci
func TestAcquireReadLockFail(t *testing.T) {
	file, fin, _ := filelocks.FileFactory()
	defer fin()

	filelocks.SetMaxLockCycles(1)
	filelocks.SetLockCycleDurationFactor(1)

	// create a channel big enough for all waiting groups
	errors := make(chan error, 8000)
	var wg sync.WaitGroup
	for i := 0; i < 8000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			l, err := filelocks.AcquireReadLock(file)
			if err != nil {
				// collect the error in a channel
				errors <- err
				return
			}
			err = filelocks.ReleaseLock(l)
			assert.Nil(t, err)
		}()
	}

	// at least one error should have occurred
	assert.NotNil(t, <-errors)

	wg.Wait()
}
*/

func TestReleaseLock(t *testing.T) {
	file, fin, _ := filelocks.FileFactory()
	defer fin()

	l1, err := filelocks.AcquireWriteLock(file)
	assert.Equal(t, true, l1.Locked())
	assert.Nil(t, err)

	err = filelocks.ReleaseLock(l1)
	assert.Nil(t, err)
	assert.Equal(t, false, l1.Locked())
}
