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

package eosfs

import (
	"context"
	"io"
	"os"
	"path"
	"strconv"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	ok, err := chunking.IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "eos: error checking path")
	}
	if ok {
		var assembledFile string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(p, r)
		if err != nil {
			return err
		}
		if p == "" {
			return errtypes.PartialContent(ref.String())
		}
		fd, err := os.Open(assembledFile)
		if err != nil {
			return errors.Wrap(err, "eos: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	fn := fs.wrap(ctx, p)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	// We need the auth corresponding to the parent directory
	// as the file might not exist at the moment
	auth, err := fs.getUserAuth(ctx, u, path.Dir(fn))
	if err != nil {
		return err
	}

	if metadata == nil {
		metadata = map[string]string{}
	}
	app := metadata["lockholder"]
	if app == "" {
		app = "reva_eosclient::write"
	} else {
		// if we have a lock context, the app for EOS must match the lock holder
		app = fs.EncodeAppName(app)
	}

	disableVersioning, err := strconv.ParseBool(metadata["disableVersioning"])
	if err != nil {
		disableVersioning = false
	}

	contentLength := metadata[ocdav.HeaderContentLength]
	len, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return errors.New("No content length specified in EOS upload, got: " + contentLength)
	}

	return fs.c.Write(ctx, auth, fn, r, len, app, disableVersioning)
}

func (fs *Eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"simple": p,
	}, nil
}
