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

//go:build ceph
// +build ceph

package cephfs

import (
	"context"
	"io"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
)

func (fs *cephfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	user := fs.makeUser(ctx)
	p := ref.GetPath()

	ok, err := IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "cephfs: error checking path")
	}
	if ok {
		var assembledFile string
		p, assembledFile, err = NewChunkHandler(ctx, fs).WriteChunk(p, r)
		if err != nil {
			return err
		}
		if p == "" {
			return errtypes.PartialContent(ref.String())
		}
		user.op(func(cv *cacheVal) {
			r, err = cv.mount.Open(assembledFile, os.O_RDONLY, 0)
		})
		if err != nil {
			return errors.Wrap(err, "cephfs: error opening assembled file")
		}
		defer r.Close()
		defer user.op(func(cv *cacheVal) {
			_ = cv.mount.Unlink(assembledFile)
		})
	}

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

	return err
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
