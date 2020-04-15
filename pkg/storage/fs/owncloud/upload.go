// Copyright 2018-2019 CERN
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

package owncloud

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	tusd "github.com/cs3org/reva/internal/http/services/dataprovider/handler"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
)

var defaultFilePerm = os.FileMode(0664)

// TODO deprecated ... use tus
func (fs *ocfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// we cannot rely on /tmp as it can live in another partition and we can
	// hit invalid cross-device link errors, so we create the tmp file in the same directory
	// the file is supposed to be written.
	tmp, err := ioutil.TempFile(path.Dir(np), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocfs: error creating tmp fn at "+path.Dir(np))
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "ocfs: error writing to tmp file "+tmp.Name())
	}

	// if destination exists
	if _, err := os.Stat(np); err == nil {
		// copy attributes of existing file to tmp file
		if err := fs.copyMD(np, tmp.Name()); err != nil {
			return errors.Wrap(err, "ocfs: error copying metadata from "+np+" to "+tmp.Name())
		}
		// create revision
		if err := fs.archiveRevision(ctx, fs.getVersionsPath(ctx, np), np); err != nil {
			return err
		}
	}

	// TODO(jfd): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), np); err != nil {
		return errors.Wrap(err, "ocfs: error renaming from "+tmp.Name()+" to "+np)
	}

	return nil
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (fs *ocfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}

// NewUpload returns an upload id that can be used for uploads with tus
// TODO read optional content for small files in this request
func (fs *ocfs) NewUpload(ctx context.Context, ref *provider.Reference, uploadLength int64) (uploadID string, err error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return "", errors.Wrap(err, "ocfs: error resolving reference")
	}

	// try generating a uuid
	// TODO remember uploadID and use as versionid?
	var newid uuid.UUID
	if newid, err = uuid.NewV4(); err != nil {
		return "", errors.Wrap(err, "ocfs: error generating upload id")
	}
	uploadID = newid.String()

	info := tusd.FileInfo{
		// store filename so tusdsvc can move there when finalizing the upload
		MetaData: tusd.MetaData{
			"filename": np,
		},
	}

	if uploadLength == 0 {
		info.SizeIsDeferred = true
	} else {
		info.Size = uploadLength
	}

	binPath, err := fs.getUploadPath(ctx, uploadID)
	if err != nil {
		return "", errors.Wrap(err, "ocfs: error resolving upload path")
	}
	info.ID = uploadID
	info.Storage = map[string]string{
		"Type": "OwnCloudStore",
		"Path": binPath,
	}

	// Create binary file with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			// try creating upload dir
			// TODO refactor this to have a single method that creates all dirs instead of spreading this all over the code.
			// the method should return a struct with all needed paths
			ud := path.Dir(binPath)
			if err := os.MkdirAll(ud, 0700); err != nil {
				return "", errors.Wrap(err, "ocfs: error creating upload dir "+ud)
			}

			// try creating upload file again
			file, err = os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
			if err != nil {
				return "", err
			}

		} else {
			return "", err
		}
	}
	defer file.Close()

	data, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return uploadID, ioutil.WriteFile(binPath+".info", data, defaultFilePerm)
}

// GetUpload returns the Upload for the given upload id
func (fs *ocfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	binPath, err := fs.getUploadPath(ctx, id)
	if err != nil {
		return nil, err
	}
	infoPath := binPath + ".info"
	info := tusd.FileInfo{}
	data, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	stat, err := os.Stat(binPath)
	if err != nil {
		return nil, err
	}

	info.Offset = stat.Size()

	return &fileUpload{
		info:     info,
		binPath:  binPath,
		infoPath: infoPath,
		fs:       fs,
	}, nil
}

// AsTerminatableUpload returnsa a TerminatableUpload
func (fs *ocfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
	return upload.(*fileUpload)
}

// AsLengthDeclarableUpload returnsa a LengthDeclarableUpload
func (fs *ocfs) AsLengthDeclarableUpload(upload tusd.Upload) tusd.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

// AsConcatableUpload returnsa a ConcatableUpload
func (fs *ocfs) AsConcatableUpload(upload tusd.Upload) tusd.ConcatableUpload {
	return upload.(*fileUpload)
}

type fileUpload struct {
	// info stores the current information about the upload
	info tusd.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
	// only fs knows how to handle metadata and versions
	fs *ocfs
}

// GetInfo returns the FileInfo
func (upload *fileUpload) GetInfo(ctx context.Context) (tusd.FileInfo, error) {
	return upload.info, nil
}

// WriteChunk writes the stream from the reader to the given offset of the upload
func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for OwnCloudStore it's not important whether the stream has ended
	// on purpose or accidentally.
	if err == io.ErrUnexpectedEOF {
		err = nil
	}

	upload.info.Offset += n
	upload.writeInfo()

	return n, err
}

// GetReader returns an io.Readerfor the upload
func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

// Terminate terminates the upload
func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.infoPath); err != nil {
		return err
	}
	if err := os.Remove(upload.binPath); err != nil {
		return err
	}
	return nil
}

// ConcatUploads concatenates multiple uploads
func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []tusd.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(fileUpload.binPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

// DeclareLength updates the upload length information
func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the target destination
func (upload *fileUpload) FinishUpload(ctx context.Context) error {

	fn := upload.info.MetaData["filename"]

	// if destination exists
	// TODO this only works when the target lives on this storage, what if we uploaded to the users upload folder but now need to move to a different storage?
	// can that happen? the storageprovider is tied to a tusdsvc ... and initiate upload should have been called on the parent folder ... which in theory already is the correct destination storage
	//
	if _, err := os.Stat(fn); err == nil {
		// copy attributes of existing file to tmp file
		if err := upload.fs.copyMD(fn, upload.binPath); err != nil {
			return errors.Wrap(err, "ocfs: error copying metadata from "+fn+" to "+upload.binPath)
		}
		// create revision
		if err := upload.fs.archiveRevision(ctx, upload.fs.getVersionsPath(ctx, fn), fn); err != nil {
			return err
		}
	}

	// TODO double check the metadata path exists
	err := os.Rename(upload.binPath, fn)

	// TODO trigger metadata propagation?
	return err
}