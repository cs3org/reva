// Copyright 2018-2023 CERN
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

package tus

import (
	"context"
	"os"
	"path/filepath"

	tusfilestore "github.com/tus/tusd/pkg/filestore"
	"github.com/tus/tusd/pkg/handler"
)

// FileStore is a wrapper around tusfilestore.FileStore that provides additional functionality.
type FileStore struct {
	tusfilestore.FileStore
}

// NewFileStore creates a new instance of FileStore with the specified path.
func NewFileStore(path string) FileStore {
	return FileStore{tusfilestore.New(path)}
}

// NewUpload creates a new upload for the given file info in the file store.
func (store FileStore) NewUpload(ctx context.Context, session Session) (handler.Upload, error) {
	err := os.MkdirAll(filepath.Dir(filepath.Join(store.Path, session.ID)), 0755)
	if err != nil {
		return nil, err
	}
	return store.FileStore.NewUpload(ctx, session.ToFileInfo())
}

// CleanupMetadata removes the metadata associated with the given ID.
func (store FileStore) CleanupMetadata(_ context.Context, id string) error {
	return os.Remove(store.infoPath(id))
}

// infoPath returns the path to the .info file storing the file's info.
func (store FileStore) infoPath(id string) string {
	return filepath.Join(store.Path, id+".info")
}
