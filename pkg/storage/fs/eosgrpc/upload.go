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
	"io"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
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
	return ref.GetPath(), nil

}
