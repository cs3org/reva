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

package eosfs

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	upload, err := fs.GetUpload(ctx, ref.GetPath())
	if err != nil {
		// Upload corresponding to this ID was not found.
		// Assume that this corresponds to the resource path to which the file has to be uploaded.

		// Set the length to 0 and set SizeIsDeferred to true
		metadata := map[string]string{"sizedeferred": "true"}
		uploadIDs, err := fs.InitiateUpload(ctx, ref, 0, metadata)
		if err != nil {
			return err
		}
		if upload, err = fs.GetUpload(ctx, uploadIDs["simple"]); err != nil {
			return errors.Wrap(err, "ocfs: error retrieving upload")
		}
	}

	p := upload.MetaData["filename"]
	ok, err := chunking.IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "eosfs: error checking path")
	}
	if ok {
		var assembledFile string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(p, r)
		if err != nil {
			return err
		}
		if p == "" {
			if err = fs.TerminateUpload(ctx, upload); err != nil {
				return errors.Wrap(err, "eosfs: error removing auxiliary files")
			}
			return errtypes.PartialContent(ref.String())
		}
		upload.MetaData["filename"] = p
		fd, err := os.Open(assembledFile)
		if err != nil {
			return errors.Wrap(err, "eosfs: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	return fs.FinishUpload(ctx, upload, r)
}

// InitiateUpload returns upload ids corresponding to different protocols it supports
// TODO: read optional content for small files in this request
func (fs *eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}
	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": fn,
		},
		ID:   uuid.New().String(),
		Size: uploadLength,
	}

	if metadata != nil {
		if metadata["mtime"] != "" {
			info.MetaData["mtime"] = metadata["mtime"]
		}
		if metadata["chunk-size"] != "" {
			info.MetaData["chunk-size"] = metadata["chunk-size"]
		}
		if metadata["total-length"] != "" {
			info.MetaData["total-length"] = metadata["total-length"]
		}
		if metadata["chunked"] != "" {
			info.MetaData["chunked"] = metadata["chunked"]
		}
		if _, ok := metadata["sizedeferred"]; ok {
			info.SizeIsDeferred = true
		}
	}

	if !info.SizeIsDeferred && info.Size == 0 {
		// no need to create info file and finish directly
		if err = fs.FinishUpload(ctx, info, nil); err != nil {
			return nil, err
		}
	} else {
		// writeInfo writes the info file to disk
		err = fs.writeInfo(ctx, info)
		if err != nil {
			return nil, err
		}
	}

	return map[string]string{
		"simple": info.ID,
	}, nil
}

func (fs *eosfs) getUploadInfoPath(ctx context.Context, uploadID string) string {
	return filepath.Join(fs.conf.CacheDirectory, fs.getLayout(ctx), "uploads", uploadID)
}

func (fs *eosfs) writeInfo(ctx context.Context, info tusd.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fs.getUploadInfoPath(ctx, info.ID), data, defaultFilePerm)
}

// GetUpload returns the Upload for the given upload id
func (fs *eosfs) GetUpload(ctx context.Context, id string) (tusd.FileInfo, error) {
	var info tusd.FileInfo
	data, err := ioutil.ReadFile(fs.getUploadInfoPath(ctx, id))
	if err != nil {
		return tusd.FileInfo{}, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return tusd.FileInfo{}, err
	}
	return info, nil
}

func (fs *eosfs) FinishUpload(ctx context.Context, upload tusd.FileInfo, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}
	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}
	err = fs.c.Write(ctx, uid, gid, upload.MetaData["filename"], r)
	if err != nil {
		return err
	}
	return fs.TerminateUpload(ctx, upload)
}

func (fs *eosfs) TerminateUpload(ctx context.Context, upload tusd.FileInfo) error {
	if err := os.Remove(fs.getUploadInfoPath(ctx, upload.ID)); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
