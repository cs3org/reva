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
	"io"
	"os"
	"path"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/pkg/errors"
)

func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, uff storage.UploadFinishedFunc) (provider.ResourceInfo, error) {
	resPath, err := fs.resolve(ctx, ref)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "eos: error resolving reference")
	}

	if chunking.IsChunked(resPath) {
		var assembledFile string
		resPath, assembledFile, err = fs.chunkHandler.WriteChunk(resPath, r)
		if err != nil {
			return provider.ResourceInfo{}, err
		}
		if resPath == "" {
			return provider.ResourceInfo{}, errtypes.PartialContent(ref.String())
		}
		fd, err := os.Open(assembledFile)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "eos: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	u, err := getUser(ctx)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "eos: no user in ctx")
	}

	// We need the auth corresponding to the parent directory
	// as the file might not exist at the moment
	auth, err := fs.getUserAuth(ctx, u, path.Dir(resPath))
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	if err := fs.c.Write(ctx, auth, resPath, r); err != nil {
		return provider.ResourceInfo{}, err
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, resPath)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	ri, err := fs.convertToResourceInfo(ctx, eosFileInfo, ref.ResourceId.GetSpaceId(), false)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	return *ri, nil
}

func (fs *eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"simple": p,
	}, nil
}
