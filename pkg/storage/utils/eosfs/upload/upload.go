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

package upload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/eosclient"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

type Upload struct {
	ID       string
	Info     tusd.FileInfo
	BinPath  string
	InfoPath string

	client eosclient.EOSClient
	log    zerolog.Logger
}

func New(ctx context.Context, info tusd.FileInfo, storageRoot string, client eosclient.EOSClient) (*Upload, error) {
	u := &Upload{
		ID:       info.ID,
		Info:     info,
		BinPath:  filepath.Join(storageRoot, info.ID),
		InfoPath: filepath.Join(storageRoot, info.ID+".info"),
		client:   client,
	}
	u.log = appctx.GetLogger(ctx).
		With().
		Interface("info", info).
		Str("binPath", u.BinPath).
		Logger()

	file, err := os.OpenFile(u.BinPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return u, u.writeInfo()
}

func Get(ctx context.Context, id, storageRoot string, client eosclient.EOSClient) (*Upload, error) {
	u := &Upload{
		ID:       id,
		Info:     tusd.FileInfo{},
		BinPath:  filepath.Join(storageRoot, id),
		InfoPath: filepath.Join(storageRoot, id+".info"),
		client:   client,
	}
	u.log = appctx.GetLogger(ctx).
		With().
		Interface("info", u.Info).
		Str("binPath", u.BinPath).
		Logger()

	data, err := os.ReadFile(u.InfoPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = tusd.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &u.Info); err != nil {
		return nil, err
	}

	return u, nil
}

func (u *Upload) writeInfo() error {
	data, err := json.Marshal(u.Info)
	if err != nil {
		return err
	}
	return os.WriteFile(u.InfoPath, data, defaultFilePerm)
}

func (u *Upload) FinishUpload(ctx context.Context) error {
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: u.Info.Storage["UID"],
			GID: u.Info.Storage["GID"],
		},
	}

	file, err := os.Open(u.BinPath)
	if err != nil {
		return err
	}
	defer file.Close()

	err = u.client.Write(ctx, auth, u.Info.Storage["Path"], file)
	if err != nil {
		return err
	}

	return u.cleanup()
}

func (u *Upload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(u.BinPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// calculate cheksum here? needed for the TUS checksum extension. https://tus.io/protocols/resumable-upload.html#checksum
	// TODO but how do we get the `Upload-Checksum`? WriteChunk() only has a context, offset and the reader ...
	// It is sent with the PATCH request, well or in the POST when the creation-with-upload extension is used
	// but the tus handler uses a context.Background() so we cannot really check the header and put it in the context ...
	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for the ocis driver it's not important whether the stream has ended
	// on purpose or accidentally.
	if err != nil && err != io.ErrUnexpectedEOF {
		return n, err
	}

	u.Info.Offset += n
	return n, u.writeInfo()
}

func (u *Upload) GetInfo(_ context.Context) (tusd.FileInfo, error) {
	return u.Info, nil
}

func (u *Upload) GetReader(_ context.Context) (io.Reader, error) {
	return os.Open(u.BinPath)
}

// Terminate terminates the upload
func (u *Upload) Terminate(_ context.Context) error {
	return u.cleanup()
}

// DeclareLength updates the upload length information
func (u *Upload) DeclareLength(_ context.Context, length int64) error {
	u.Info.Size = length
	u.Info.SizeIsDeferred = false
	return u.writeInfo()
}

// ConcatUploads concatenates multiple uploads
func (u *Upload) ConcatUploads(_ context.Context, uploads []tusd.Upload) (err error) {
	file, err := os.OpenFile(u.BinPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*Upload)

		src, err := os.Open(fileUpload.BinPath)
		if err != nil {
			return err
		}
		defer src.Close()

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (u *Upload) cleanup() error {
	var e error
	if err := os.Remove(u.BinPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e = err
		u.log.Error().Str("path", u.BinPath).Err(err).Msg("removing upload failed")
	}
	if err := os.Remove(u.InfoPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e = err
		u.log.Error().Str("path", u.InfoPath).Err(err).Msg("removing upload info failed")
	}
	return e
}
