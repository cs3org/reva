// Copyright 2018-2020 CERN
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
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
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
	tmp, err := ioutil.TempFile(filepath.Dir(np), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocfs: error creating tmp fn at "+filepath.Dir(np))
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

// InitiateUpload returns an upload id that can be used for uploads with tus
// TODO read optional content for small files in this request
func (fs *ocfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64) (uploadID string, err error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return "", errors.Wrap(err, "ocfs: error resolving reference")
	}

	p := fs.unwrap(ctx, np)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(p),
			"dir":      filepath.Dir(p),
		},
		Size: uploadLength,
	}

	upload, err := fs.NewUpload(ctx, info)
	if err != nil {
		return "", err
	}

	info, _ = upload.GetInfo(ctx)

	return info.ID, nil
}

// UseIn tells the tus upload middleware which extensions it supports.
func (fs *ocfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}

// To implement the core tus.io protocol as specified in https://tus.io/protocols/resumable-upload.html#core-protocol
// - the storage needs to implement NewUpload and GetUpload
// - the upload needs to implement the tusd.Upload interface: WriteChunk, GetInfo, GetReader and FinishUpload

func (fs *ocfs) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("ocfs: NewUpload")

	fn := info.MetaData["filename"]
	if fn == "" {
		return nil, errors.New("ocfs: missing filename in metadata")
	}
	info.MetaData["filename"] = filepath.Clean(info.MetaData["filename"])

	dir := info.MetaData["dir"]
	if dir == "" {
		return nil, errors.New("ocfs: missing dir in metadata")
	}
	info.MetaData["dir"] = filepath.Clean(info.MetaData["dir"])

	np := fs.wrap(ctx, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]))

	log.Debug().Interface("info", info).Msg("ocfs: resolved filename")

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving upload path")
	}
	info.Storage = map[string]string{
		"Type":                "OwnCloudStore",
		"InternalDestination": np,
	}
	// Create binary file in the upload folder with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	u := &fileUpload{
		info:     info,
		binPath:  binPath,
		infoPath: binPath + ".info",
		fs:       fs,
	}

	// writeInfo creates the file by itself if necessary
	err = u.writeInfo()
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (fs *ocfs) getUploadPath(ctx context.Context, uploadID string) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return "", err
	}
	return filepath.Join(fs.c.DataDirectory, u.Username, "uploads", uploadID), nil
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
	if err != nil {
		if err != io.ErrUnexpectedEOF {
			return n, err
		}
	}

	upload.info.Offset += n
	err = upload.writeInfo() // TODO info is written here ... we need to truncate in DiscardChunk

	return n, err
}

// GetReader returns an io.Readerfor the upload
func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *fileUpload) FinishUpload(ctx context.Context) error {

	/*
		checksum := upload.info.MetaData["checksum"]
		if checksum != "" {
			// TODO check checksum
			s := strings.SplitN(checksum, " ", 2)
			if len(s) == 2 {
				alg, hash := s[0], s[1]

			}
		}
	*/

	np := upload.info.Storage["InternalDestination"]

	// if destination exists
	// TODO check etag with If-Match header
	if _, err := os.Stat(np); err == nil {
		// copy attributes of existing file to tmp file
		if err := upload.fs.copyMD(np, upload.binPath); err != nil {
			return errors.Wrap(err, "ocfs: error copying metadata from "+np+" to "+upload.binPath)
		}
		// create revision
		if err := upload.fs.archiveRevision(ctx, upload.fs.getVersionsPath(ctx, np), np); err != nil {
			return err
		}
	}

	err := os.Rename(upload.binPath, np)
	// TODO else remove info? or leave as history?

	// metadata propagation is left to the storage implementation
	return err
}

// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// - the storage needs to implement AsTerminatableUpload
// - the upload needs to implement Terminate

// AsTerminatableUpload returnsa a TerminatableUpload
func (fs *ocfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
	return upload.(*fileUpload)
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

// To implement the creation-defer-length extension as specified in https://tus.io/protocols/resumable-upload.html#creation
// - the storage needs to implement AsLengthDeclarableUpload
// - the upload needs to implement DeclareLength

// AsLengthDeclarableUpload returnsa a LengthDeclarableUpload
func (fs *ocfs) AsLengthDeclarableUpload(upload tusd.Upload) tusd.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

// DeclareLength updates the upload length information
func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// To implement the concatenation extension as specified in https://tus.io/protocols/resumable-upload.html#concatenation
// - the storage needs to implement AsConcatableUpload
// - the upload needs to implement ConcatUploads

// AsConcatableUpload returnsa a ConcatableUpload
func (fs *ocfs) AsConcatableUpload(upload tusd.Upload) tusd.ConcatableUpload {
	return upload.(*fileUpload)
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
