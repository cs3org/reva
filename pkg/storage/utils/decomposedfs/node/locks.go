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

package node

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/filelocks"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

// SetLock sets a lock on the node
func (n *Node) SetLock(ctx context.Context, lock *provider.Lock) error {
	lockFilePath := n.LockFilePath()
	// check existing lock

	if l, _ := n.ReadLock(ctx); l != nil {
		lockID, _ := ctxpkg.ContextGetLockID(ctx)
		if l.LockId != lockID {
			return errtypes.Locked(l.LockId)
		}

		err := os.Remove(lockFilePath)
		if err != nil {
			return err
		}
	}

	// ensure parent path exists
	if err := os.MkdirAll(filepath.Dir(lockFilePath), 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating parent folder for lock")
	}
	fileLock, err := filelocks.AcquireWriteLock(n.InternalPath())

	if err != nil {
		return err
	}

	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	// O_EXCL to make open fail when the file already exists
	f, err := os.OpenFile(lockFilePath, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: could not create lock file")
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(lock); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not write lock file")
	}

	return err
}

// ReadLock reads the lock id for a node
func (n Node) ReadLock(ctx context.Context) (*provider.Lock, error) {

	// ensure parent path exists
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error creating parent folder for lock")
	}
	fileLock, err := filelocks.AcquireReadLock(n.InternalPath())

	if err != nil {
		return nil, err
	}

	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	f, err := os.Open(n.LockFilePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errtypes.NotFound("no lock found")
		}
		return nil, errors.Wrap(err, "Decomposedfs: could not open lock file")
	}
	defer f.Close()

	lock := &provider.Lock{}
	if err := json.NewDecoder(f).Decode(lock); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("Decomposedfs: could not decode lock file, ignoring")
		return nil, errors.Wrap(err, "Decomposedfs: could not read lock file")
	}
	return lock, err
}

// RefreshLock refreshes the node's lock
func (n *Node) RefreshLock(ctx context.Context, lock *provider.Lock) error {

	// ensure parent path exists
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating parent folder for lock")
	}
	fileLock, err := filelocks.AcquireWriteLock(n.InternalPath())

	if err != nil {
		return err
	}

	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	f, err := os.OpenFile(n.LockFilePath(), os.O_RDWR, os.ModeExclusive)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errtypes.PreconditionFailed("lock does not exist")
	case err != nil:
		return errors.Wrap(err, "Decomposedfs: could not open lock file")
	}
	defer f.Close()

	oldLock := &provider.Lock{}
	if err := json.NewDecoder(f).Decode(oldLock); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not read lock")
	}

	// check lock
	if oldLock.LockId != lock.LockId {
		return errtypes.PreconditionFailed("mismatching lock")
	}

	u := ctxpkg.ContextMustGetUser(ctx)
	if !utils.UserEqual(oldLock.User, u.Id) {
		return errtypes.PermissionDenied("cannot refresh lock of another holder")
	}

	if !utils.UserEqual(oldLock.User, lock.GetUser()) {
		return errtypes.PermissionDenied("cannot change holder when refreshing a lock")
	}

	if err := json.NewEncoder(f).Encode(lock); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not write lock file")
	}

	return err
}

// Unlock unlocks the node
func (n *Node) Unlock(ctx context.Context, lock *provider.Lock) error {

	// ensure parent path exists
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating parent folder for lock")
	}
	fileLock, err := filelocks.AcquireWriteLock(n.InternalPath())

	if err != nil {
		return err
	}

	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	f, err := os.OpenFile(n.LockFilePath(), os.O_RDONLY, os.ModeExclusive)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errtypes.PreconditionFailed("lock does not exist")
	case err != nil:
		return errors.Wrap(err, "Decomposedfs: could not open lock file")
	}
	defer f.Close()

	oldLock := &provider.Lock{}
	if err := json.NewDecoder(f).Decode(oldLock); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not read lock")
	}

	// check lock
	if lock == nil || (oldLock.LockId != lock.LockId) {
		return errtypes.Locked(oldLock.LockId)
	}

	u := ctxpkg.ContextMustGetUser(ctx)
	if !utils.UserEqual(oldLock.User, u.Id) {
		return errtypes.PermissionDenied("mismatching holder")
	}

	if err = os.Remove(f.Name()); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not remove lock file")
	}
	return err
}

// CheckLock compares the context lock with the node lock
func (n *Node) CheckLock(ctx context.Context) error {
	lockID, _ := ctxpkg.ContextGetLockID(ctx)
	lock, _ := n.ReadLock(ctx)
	if lock != nil {
		switch lockID {
		case "":
			return errtypes.Locked(lock.LockId) // no lockid in request
		case lock.LockId:
			return nil // ok
		default:
			return errtypes.PreconditionFailed("mismatching lock")
		}
	}
	if lockID != "" {
		return errtypes.PreconditionFailed("not locked")
	}
	return nil // ok
}

func readLocksIntoOpaque(ctx context.Context, n *Node, ri *provider.ResourceInfo) error {

	// ensure parent path exists
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating parent folder for lock")
	}
	fileLock, err := filelocks.AcquireReadLock(n.InternalPath())

	if err != nil {
		return err
	}

	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	f, err := os.Open(n.LockFilePath())
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("Decomposedfs: could not open lock file")
		return err
	}
	defer f.Close()

	lock := &provider.Lock{}
	if err := json.NewDecoder(f).Decode(lock); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("Decomposedfs: could not read lock file")
	}

	// reencode to ensure valid json
	var b []byte
	if b, err = json.Marshal(lock); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("Decomposedfs: could not marshal locks")
	}
	if ri.Opaque == nil {
		ri.Opaque = &types.Opaque{
			Map: map[string]*types.OpaqueEntry{},
		}
	}
	ri.Opaque.Map["lock"] = &types.OpaqueEntry{
		Decoder: "json",
		Value:   b,
	}
	return err
}

func (n *Node) hasLocks(ctx context.Context) bool {
	_, err := os.Stat(n.LockFilePath()) // FIXME better error checking
	return err == nil
}
