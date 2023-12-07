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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// See the handler.DataStore interface for documentation about the different
// methods.
type FileStore struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
}

// New creates a new file based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func NewFileStore(path string) FileStore {
	return FileStore{path}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store FileStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
}

func (store FileStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	return nil, fmt.Errorf("fileStore: must call NewUploadSession")
}
func (store FileStore) NewUploadWithSession(ctx context.Context, session Session) (handler.Upload, error) {

	if session.ID == "" {
		return nil, fmt.Errorf("s3store: upload id must be set")
	}

	binPath := store.binPath(session.ID)
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return nil, err
	}

	session.Storage = map[string]string{
		"Type": "filestore",
		"Path": binPath,
	}

	// Create binary file with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("upload directory does not exist: %s", store.Path)
		}
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}

	upload := &fileUpload{session.ID, &store, &session}

	err = session.Persist(ctx)
	if err != nil {
		return nil, err
	}

	return upload, nil
}

func (store FileStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	return &fileUpload{
		id:    id,
		store: &store,
	}, nil
	/*
		info := handler.FileInfo{}
		data, err := ioutil.ReadFile(store.infoPath(id))
		if err != nil {
			if os.IsNotExist(err) {
				// Interpret os.ErrNotExist as 404 Not Found
				err = handler.ErrNotFound
			}
			return nil, err
		}
		if err := json.Unmarshal(data, &info); err != nil {
			return nil, err
		}

		binPath := store.binPath(id)
		infoPath := store.infoPath(id)
		stat, err := os.Stat(binPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Interpret os.ErrNotExist as 404 Not Found
				err = handler.ErrNotFound
			}
			return nil, err
		}

		info.Offset = stat.Size()

		return &fileUpload{
			info:     info,
			binPath:  binPath,
			infoPath: infoPath,
		}, nil
	*/
}

func (store FileStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store FileStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store FileStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

// binPath returns the path to the file storing the binary data.
func (store FileStore) binPath(uploadID string) string {
	// uploadID is of the format <spaceID>:<uploadID>
	parts := strings.SplitN(uploadID, ":", 2)
	return filepath.Clean(filepath.Join(store.Path, "spaces", lookup.Pathify(parts[0], 1, 2), "blobs", lookup.Pathify(parts[1], 4, 2)))
}

type fileUpload struct {
	id    string
	store *FileStore

	// session stores the upload's current Session struct. It may be nil if it hasn't
	// been fetched yet from S3. Never read or write to it directly but instead use
	// the GetInfo and writeInfo functions.
	session *Session
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	if upload.session != nil {
		return upload.session.ToFileInfo(), nil
	}

	session, err := upload.GetSession(ctx)
	if err != nil {
		return handler.FileInfo{}, err
	}
	return session.ToFileInfo(), nil
}

func (upload *fileUpload) GetSession(ctx context.Context) (Session, error) {
	session, err := upload.fetchSession(ctx)
	if err != nil {
		return session, err
	}

	upload.session = &session

	return session, nil
}
func (upload *fileUpload) fetchSession(ctx context.Context) (Session, error) {
	id := upload.id
	store := upload.store

	// Get file info stored in separate object
	session, err := ReadSession(ctx, store.Path, id)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.store.binPath(upload.id), os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	upload.session.Offset += n
	return n, err
}

func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.store.binPath(upload.id))
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.store.sessionPath(upload.id)); err != nil {
		return err
	}
	if err := os.Remove(upload.store.binPath(upload.id)); err != nil {
		return err
	}
	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := os.OpenFile(upload.store.binPath(upload.id), os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(upload.store.binPath(fileUpload.id))
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.session.Size = length
	upload.session.SizeIsDeferred = false
	return upload.session.Persist(ctx)
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	return nil
}

// infoPath returns the path to the .info file storing the file's info.
func (store FileStore) sessionPath(id string) string {
	return filepath.Join(store.Path, id+".mpk")
}
