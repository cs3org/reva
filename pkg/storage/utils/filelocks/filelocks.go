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

package filelocks

import (
	"errors"
	"os"

	"github.com/gofrs/flock"
)

// flockFile returns the flock filename for a given file name
// it returns an empty string if the input is empty
func FlockFile(file string) string {
	var n string
	if len(file) > 0 {
		n = file + ".flock"
	}
	return n
}

// acquireWriteLog acquires a lock on a file or directory.
// if the parameter write is true, it gets an exclusive write lock, otherwise a shared read lock.
// The function returns a Flock object, unlocking has to be done in the calling function.
func acquireLock(file string, write bool) (*flock.Flock, error) {
	var err error

	// Create the a file to carry the log
	n := FlockFile(file)
	if len(n) == 0 {
		return nil, errors.New("lock path is empty")
	}
	// Acquire the write log on the target node first.
	lock := flock.New(n)

	if write {
		_, err = lock.TryLock()
	} else {
		_, err = lock.TryRLock()
	}

	if err != nil {
		return nil, err
	}
	return lock, nil
}

// AcquireReadLock tries to acquire a shared lock to read from the
// file and returns a lock object or an error accordingly.
func AcquireReadLock(file string) (*flock.Flock, error) {
	return acquireLock(file, false)
}

// AcquireWriteLock tries to acquire a shared lock to write from the
// file and returns a lock object or an error accordingly.
func AcquireWriteLock(file string) (*flock.Flock, error) {
	return acquireLock(file, true)
}

func ReleaseLock(lock *flock.Flock) error {
	// there is a probability that if the file can not be unlocked,
	// we also can not remove the file. We will only try to remove if it
	// was successfully unlocked.
	err := lock.Unlock()
	if err == nil {
		err = os.Remove(lock.Path())
	}
	return err
}
