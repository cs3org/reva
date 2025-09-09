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

//go:build ceph

package cephfs

import (
	"context"
	"io"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/errors"
)

func (fs *cephfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	user := fs.makeUser(ctx)
	p := ref.GetPath()

	// TODO(lopresti) validate lock metadata if present

	ok, err := IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "cephfs: error checking if path is chunked")
	}

	if !ok {
		var file io.WriteCloser
		user.op(func(cv *cacheVal) {
			file, err = cv.mount.Open(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.conf.FilePerms)
			if err != nil {
				err = errors.Wrap(err, "cephfs: error opening binary file")
				return
			}
			defer file.Close()

			_, err = io.Copy(file, r)
			if err != nil {
				err = errors.Wrap(err, "cephfs: error writing to binary file")
				return
			}
		})

		return nil
	}

	// upload is chunked

	var assembledFile string

	// iniate the chunk handler
	originalFilename, assembledFile, err := NewChunkHandler(ctx, fs).WriteChunk(p, r)
	if err != nil {
		return errors.Wrapf(err, "error writing chunk %v %v %v", p, r, assembledFile)
	}
	if originalFilename == "" { // means we wrote a chunk only
		return errtypes.PartialContent(ref.String())
	}
	user.op(func(cv *cacheVal) {
		err = cv.mount.Rename(assembledFile, originalFilename)
	})
	if err != nil {
		return errors.Wrap(err, "cephfs: error renaming assembled file")
	}
	defer user.op(func(cv *cacheVal) {
		_ = cv.mount.Unlink(assembledFile)
	})
	return nil

}

func (fs *cephfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	user := fs.makeUser(ctx)
	np, err := user.resolveRef(ref)
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error resolving reference")
	}

	return map[string]string{
		"simple": np,
	}, nil
}
