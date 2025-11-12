// Copyright 2018-2024 CERN
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

package cephmount

import (
	"context"
	"io"
	"os"
	"path/filepath"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/pkg/xattr"
)

// getUserInfo extracts user information for logging
func (fs *cephmountfs) getUserInfo(ctx context.Context) (username string, uid int, threadID int) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return "nobody", fs.conf.NobodyUID, 0
	}

	username = user.Username
	uid, _ = fs.threadPool.mapUserToUIDGID(user)

	// Get thread ID from the thread pool
	thread, err := fs.threadPool.getExistingUserThread(uid)
	if err == nil && thread != nil {
		threadID = thread.threadID
	}

	return username, uid, threadID
}

// logOperation creates a debug log entry with consistent user and thread context
func (fs *cephmountfs) logOperation(ctx context.Context, operation, path string) {
	username, uid, threadID := fs.getUserInfo(ctx)

	log := appctx.GetLogger(ctx)
	log.Debug().
		Str("operation", operation).
		Str("path", path).
		Str("username", username).
		Int("uid", uid).
		Int("thread_id", threadID).
		Msg("cephmount operation")
}

// logOperationWithPaths creates a debug log entry with both received and ceph volume paths
func (fs *cephmountfs) logOperationWithPaths(ctx context.Context, operation, receivedPath, chrootPath string) {
	username, uid, threadID := fs.getUserInfo(ctx)

	// Calculate the full filesystem path
	var fullPath string
	if chrootPath == "." {
		fullPath = fs.chrootDir
	} else {
		fullPath = filepath.Join(fs.chrootDir, chrootPath)
	}

	log := appctx.GetLogger(ctx)
	log.Info().
		Str("operation", operation).
		Str("received_path", receivedPath).
		Str("chroot_path", chrootPath).
		Str("full_filesystem_path", fullPath).
		Str("ceph_volume_path", fs.cephVolumePath).
		Str("local_mount_point", fs.localMountPoint).
		Str("username", username).
		Int("uid", uid).
		Int("thread_id", threadID).
		Msg("cephmount operation with path details")
}

// logOperationError creates an error log entry with consistent user and thread context
func (fs *cephmountfs) logOperationError(ctx context.Context, operation, path string, err error) {
	username, uid, threadID := fs.getUserInfo(ctx)

	log := appctx.GetLogger(ctx)
	log.Error().
		Err(err).
		Str("operation", operation).
		Str("path", path).
		Str("username", username).
		Int("uid", uid).
		Int("thread_id", threadID).
		Msg("cephmount operation failed")
}

// executeAsUser runs a function that returns a result on the user's thread with correct UID
func (fs *cephmountfs) executeAsUser(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		// Create a nobody user for fallback operations instead of using root
		user = fs.createNobodyUser()
	}

	return fs.threadPool.ExecuteOnUserThread(ctx, user, fn)
}

// createNobodyUser creates a synthetic user representing the nobody user
func (fs *cephmountfs) createNobodyUser() *userv1beta1.User {
	return &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "cephmount-nobody",
			OpaqueId: "nobody",
		},
		Username:    "nobody",
		DisplayName: "Nobody User",
	}
}

// executeOnUserThread runs a function on the user's thread with correct UID
func (fs *cephmountfs) executeOnUserThread(ctx context.Context, fn func() error) error {
	_, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

// createDirectoryAsUser creates a directory on the user's thread with correct UID
func (fs *cephmountfs) createDirectoryAsUser(ctx context.Context, path string, perm os.FileMode) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.MkdirAll(path, perm)
	})
}

// createFileAsUser creates a file on the user's thread with correct UID
func (fs *cephmountfs) createFileAsUser(ctx context.Context, path string, perm os.FileMode) error {
	return fs.executeOnUserThread(ctx, func() error {
		file, err := fs.rootFS.OpenFile(path, os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		return file.Close()
	})
}

// statAsUser performs a stat operation on the user's thread with correct UID
func (fs *cephmountfs) statAsUser(ctx context.Context, path string) (os.FileInfo, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return fs.rootFS.Stat(path)
	})
	if err != nil {
		return nil, err
	}
	return result.(os.FileInfo), nil
}

// removeAsUser removes a file on the user's thread with correct UID
func (fs *cephmountfs) removeAsUser(ctx context.Context, path string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.Remove(path)
	})
}

// removeAllAsUser removes a directory and all its contents on the user's thread with correct UID
func (fs *cephmountfs) removeAllAsUser(ctx context.Context, path string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.RemoveAll(path)
	})
}

// renameAsUser renames a file/directory on the user's thread with correct UID
func (fs *cephmountfs) renameAsUser(ctx context.Context, oldPath, newPath string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.Rename(oldPath, newPath)
	})
}

// readDirectoryAsUser reads a directory on the user's thread with correct UID
func (fs *cephmountfs) readDirectoryAsUser(ctx context.Context, path string) ([]os.FileInfo, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		dir, err := fs.rootFS.Open(path)
		if err != nil {
			return nil, err
		}
		defer dir.Close()

		return dir.Readdir(-1)
	})
	if err != nil {
		return nil, err
	}
	return result.([]os.FileInfo), nil
}

// openFileAsUser opens a file for reading on the user's thread with correct UID
func (fs *cephmountfs) openFileAsUser(ctx context.Context, path string) (*os.File, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return fs.rootFS.Open(path)
	})
	if err != nil {
		return nil, err
	}
	return result.(*os.File), nil
}

// uploadFileAsUser creates and uploads a file on the user's thread with correct UID
func (fs *cephmountfs) uploadFileAsUser(ctx context.Context, path string, r io.ReadCloser, perm os.FileMode) error {
	return fs.executeOnUserThread(ctx, func() error {
		file, err := fs.rootFS.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(file, r)
		return err
	})
}

// setXattrAsUser sets an extended attribute on the user's thread with correct UID
func (fs *cephmountfs) setXattrAsUser(ctx context.Context, path, key string, data []byte) error {
	return fs.executeOnUserThread(ctx, func() error {
		fullPath := filepath.Join(fs.chrootDir, path)
		return xattr.Set(fullPath, key, data)
	})
}

// getXattrAsUser gets an extended attribute on the user's thread with correct UID
func (fs *cephmountfs) getXattrAsUser(ctx context.Context, path, key string) ([]byte, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		fullPath := filepath.Join(fs.chrootDir, path)
		return xattr.Get(fullPath, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// removeXattrAsUser removes an extended attribute on the user's thread with correct UID
func (fs *cephmountfs) removeXattrAsUser(ctx context.Context, path, key string) error {
	return fs.executeOnUserThread(ctx, func() error {
		fullPath := filepath.Join(fs.chrootDir, path)
		return xattr.Remove(fullPath, key)
	})
}

// listXattrsAsUser lists extended attributes on the user's thread with correct UID
func (fs *cephmountfs) listXattrsAsUser(ctx context.Context, path string) ([]string, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		fullPath := filepath.Join(fs.chrootDir, path)
		return xattr.List(fullPath)
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}
