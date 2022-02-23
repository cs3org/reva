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
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// Locks stores the local Flock structs in a map by their file names.
// That is needed because each of the struct contains a mutex that the
// gofrs/flock module is using. Thus, there must only be one Flock struct
// per file.
type Locks struct {
	mu sync.Mutex
	//
	_locks map[string]*flock.Flock
}

var localLocks Locks

func init() {
	localLocks._locks = make(map[string]*flock.Flock)
}

// getMutexedFlock returns a new Flock struct for the given file.
// If there is already one in the local store, it returns nil.
// The caller has to wait until it can get a new one out of this
// mehtod.
func getMutexedFlock(file string) *flock.Flock {
	// the local data structure to keep the lock structs is mutex protected so that
	// only one routine can access it.
	localLocks.mu.Lock()
	defer localLocks.mu.Unlock()

	// Is there lock already?
	if _, ok := localLocks._locks[file]; ok {
		// There is already a lock for this file, another can not be acquired
		return nil
	}

	// Acquire the write log on the target node first.
	localLocks._locks[file] = flock.New(file)
	return localLocks._locks[file]

}

// releaseMutexedFlock releases a Flock object that was acquired
// before by the getMutexedFlock function.
func releaseMutexedFlock(file string) {
	if len(file) == 0 {
		return
	}

	localLocks.mu.Lock()
	defer localLocks.mu.Unlock()

	_, ok := localLocks._locks[file]
	if ok {
		delete(localLocks._locks, file)
	}
}

// acquireWriteLog acquires a lock on a file or directory.
// if the parameter write is true, it gets an exclusive write lock, otherwise a shared read lock.
// The function returns a Flock object, unlocking has to be done in the calling function.
func acquireLock(file string, write bool) (*flock.Flock, error) {
	var err error

	// Create a file to carry the log
	n := FlockFile(file)
	if len(n) == 0 {
		return nil, errors.New("lock path is empty")
	}

	var flock *flock.Flock
	for i := 1; i <= 10; i++ {
		if flock = getMutexedFlock(n); flock != nil {
			break
		}
		w := time.Duration(i*3) * time.Millisecond
		// fmt.Printf("Waiting for lock to release %d\n", w)

		time.Sleep(w)
	}
	if flock == nil {
		return nil, errors.New("unable to acquire a lock on the file")
	}

	var ok bool
	for i := 1; i <= 10; i++ {
		if write {
			ok, err = flock.TryLock()
		} else {
			ok, err = flock.TryRLock()
		}

		if ok {
			break
		}

		time.Sleep(time.Duration(i*3) * time.Millisecond)
	}

	if !ok {
		err = errors.New("could not acquire lock after wait")
	}

	if err != nil {
		return nil, err
	}
	// fmt.Printf("Returning flock for %s\n", flock.Path())
	return flock, nil
}

// FlockFile returns the flock filename for a given file name
// it returns an empty string if the input is empty
func FlockFile(file string) string {
	var n string
	if len(file) > 0 {
		n = file + ".flock"
	}
	return n
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

// ReleaseLock releases a lock from a file that was previously created
// by AcquireReadLock or AcquireWriteLock.
func ReleaseLock(lock *flock.Flock) error {
	// there is a probability that if the file can not be unlocked,
	// we also can not remove the file. We will only try to remove if it
	// was successfully unlocked.
	var err error
	n := lock.Path()
	// There is already a lock for this file

	err = lock.Unlock()
	if err == nil {
		err = os.Remove(n)
		// fmt.Printf("Removing flock for %s\n", n)

	}
	releaseMutexedFlock(n)

	return err
}
