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

package nceph

import (
	"context"
	"io"
	"os"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/pkg/xattr"
)

// executeAsUser runs a function that returns a result on the user's thread with correct UID
func (fs *ncephfs) executeAsUser(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		// Create a nobody user for fallback operations instead of using root
		user = fs.createNobodyUser()
	}

	return fs.threadPool.ExecuteOnUserThread(ctx, user, fn)
}

// createNobodyUser creates a synthetic user representing the nobody user
func (fs *ncephfs) createNobodyUser() *userv1beta1.User {
	return &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "nceph-nobody",
			OpaqueId: "nobody",
		},
		Username:    "nobody",
		DisplayName: "Nobody User",
	}
}

// executeOnUserThread runs a function on the user's thread with correct UID
func (fs *ncephfs) executeOnUserThread(ctx context.Context, fn func() error) error {
	_, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

// createDirectoryAsUser creates a directory on the user's thread with correct UID
func (fs *ncephfs) createDirectoryAsUser(ctx context.Context, path string, perm os.FileMode) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.MkdirAll(path, perm)
	})
}

// createFileAsUser creates a file on the user's thread with correct UID
func (fs *ncephfs) createFileAsUser(ctx context.Context, path string, perm os.FileMode) error {
	return fs.executeOnUserThread(ctx, func() error {
		file, err := fs.rootFS.OpenFile(path, os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		return file.Close()
	})
}

// statAsUser performs a stat operation on the user's thread with correct UID
func (fs *ncephfs) statAsUser(ctx context.Context, path string) (os.FileInfo, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return fs.rootFS.Stat(path)
	})
	if err != nil {
		return nil, err
	}
	return result.(os.FileInfo), nil
}

// removeAsUser removes a file on the user's thread with correct UID
func (fs *ncephfs) removeAsUser(ctx context.Context, path string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.Remove(path)
	})
}

// removeAllAsUser removes a directory and all its contents on the user's thread with correct UID
func (fs *ncephfs) removeAllAsUser(ctx context.Context, path string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.RemoveAll(path)
	})
}

// renameAsUser renames a file/directory on the user's thread with correct UID
func (fs *ncephfs) renameAsUser(ctx context.Context, oldPath, newPath string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return fs.rootFS.Rename(oldPath, newPath)
	})
}

// readDirectoryAsUser reads a directory on the user's thread with correct UID
func (fs *ncephfs) readDirectoryAsUser(ctx context.Context, path string) ([]os.FileInfo, error) {
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
func (fs *ncephfs) openFileAsUser(ctx context.Context, path string) (*os.File, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return fs.rootFS.Open(path)
	})
	if err != nil {
		return nil, err
	}
	return result.(*os.File), nil
}

// uploadFileAsUser creates and uploads a file on the user's thread with correct UID
func (fs *ncephfs) uploadFileAsUser(ctx context.Context, path string, r io.ReadCloser, perm os.FileMode) error {
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
func (fs *ncephfs) setXattrAsUser(ctx context.Context, path, key string, data []byte) error {
	return fs.executeOnUserThread(ctx, func() error {
		return xattr.Set(path, key, data)
	})
}

// getXattrAsUser gets an extended attribute on the user's thread with correct UID
func (fs *ncephfs) getXattrAsUser(ctx context.Context, path, key string) ([]byte, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return xattr.Get(path, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// removeXattrAsUser removes an extended attribute on the user's thread with correct UID
func (fs *ncephfs) removeXattrAsUser(ctx context.Context, path, key string) error {
	return fs.executeOnUserThread(ctx, func() error {
		return xattr.Remove(path, key)
	})
}

// listXattrsAsUser lists extended attributes on the user's thread with correct UID
func (fs *ncephfs) listXattrsAsUser(ctx context.Context, path string) ([]string, error) {
	result, err := fs.executeAsUser(ctx, func() (interface{}, error) {
		return xattr.List(path)
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}
