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

package eosgrpc

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// TODO deprecated ... use tus
func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot upload under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	return fs.c.Write(ctx, u.Username, fn, r)
}

// InitiateUpload returns an upload id that can be used for uploads with tus
// TODO read optional content for small files in this request
func (fs *eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (uploadID string, err error) {
	u, err := getUser(ctx)
	if err != nil {
		return "", errors.Wrap(err, "eos: no user in ctx")
	}

	np, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return "", errors.Wrap(err, "eos: error resolving reference")
	}

	p := fs.wrap(ctx, np)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(p),
			"dir":      filepath.Dir(p),
		},
		Size: uploadLength,
	}

	if metadata != nil && metadata["mtime"] != "" {
		info.MetaData["mtime"] = metadata["mtime"]
	}

	upload, err := fs.NewUpload(ctx, info)
	if err != nil {
		return "", err
	}

	info, _ = upload.GetInfo(ctx)

	return info.ID, nil
}

// UseIn tells the tus upload middleware which extensions it supports.
func (fs *eosfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
}

// NewUpload creates a new upload using the size as the file's length. To determine where to write the binary data
// the Fileinfo metadata must contain a dir and a filename.
// returns a unique id which is used to identify the upload. The properties Size and MetaData will be filled.
func (fs *eosfs) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("eos: NewUpload")

	fn := info.MetaData["filename"]
	if fn == "" {
		return nil, errors.New("eos: missing filename in metadata")
	}
	info.MetaData["filename"] = filepath.Clean(info.MetaData["filename"])

	dir := info.MetaData["dir"]
	if dir == "" {
		return nil, errors.New("eos: missing dir in metadata")
	}
	info.MetaData["dir"] = filepath.Clean(info.MetaData["dir"])

	log.Debug().Interface("info", info).Msg("eos: resolved filename")

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving upload path")
	}
	user, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}
	info.Storage = map[string]string{
		"Type":     "EOSStore",
		"Username": user.Username,
	}
	// Create binary file with no content

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

	if !info.SizeIsDeferred && info.Size == 0 {
		log.Debug().Interface("info", info).Msg("eos: finishing upload for empty file")
		// no need to create info file and finish directly
		err := u.FinishUpload(ctx)
		if err != nil {
			return nil, err
		}
		return u, nil
	}

	// writeInfo creates the file by itself if necessary
	err = u.writeInfo()
	if err != nil {
		return nil, err
	}

	return u, nil
}

// TODO use a subdirectory in the shadow tree
func (fs *eosfs) getUploadPath(ctx context.Context, uploadID string) (string, error) {
	return filepath.Join(fs.conf.CacheDirectory, uploadID), nil
}

// GetUpload returns the Upload for the given upload id
func (fs *eosfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
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
	fs *eosfs
}

// GetInfo returns the FileInfo
func (upload *fileUpload) GetInfo(ctx context.Context) (tusd.FileInfo, error) {
	return upload.info, nil
}

// GetReader returns an io.Reader for the upload
func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

// WriteChunk writes the stream from the reader to the given offset of the upload
// TODO use the grpc api to directly stream to a temporary uploads location in the eos shadow tree
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
	err = upload.writeInfo()

	return n, err
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

	checksum := upload.info.MetaData["checksum"]
	if checksum != "" {
		// check checksum
		s := strings.SplitN(checksum, " ", 2)
		if len(s) == 2 {
			alg, hash := s[0], s[1]

			log := appctx.GetLogger(ctx)
			log.Debug().
				Interface("info", upload.info).
				Str("alg", alg).
				Str("hash", hash).
				Msg("eos: TODO check checksum") // TODO this is done by eos if we write chunks to it directly

		}
	}
	np := filepath.Join(upload.info.MetaData["dir"], upload.info.MetaData["filename"])

	err := upload.fs.c.WriteFile(ctx, upload.info.Storage["Username"], np, upload.binPath)

	// only delete the upload if it was successfully written to eos
	if err == nil {
		// cleanup in the background, delete might take a while and we don't need to wait for it to finish
		go func() {
			if err := os.Remove(upload.infoPath); err != nil {
				if !os.IsNotExist(err) {
					log := appctx.GetLogger(ctx)
					log.Err(err).Interface("info", upload.info).Msg("eos: could not delete upload info")
				}
			}
			if err := os.Remove(upload.binPath); err != nil {
				if !os.IsNotExist(err) {
					log := appctx.GetLogger(ctx)
					log.Err(err).Interface("info", upload.info).Msg("eos: could not delete upload binary")
				}
			}
		}()
	}

	// TODO: set mtime if specified in metadata

	// metadata propagation is left to the storage implementation
	return err
}

// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// - the storage needs to implement AsTerminatableUpload
// - the upload needs to implement Terminate

// AsTerminatableUpload returns a TerminatableUpload
func (fs *eosfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
	return upload.(*fileUpload)
}

// Terminate terminates the upload
func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(upload.binPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
