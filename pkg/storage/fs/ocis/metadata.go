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

package ocis

import (
	"context"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

func (fs *ocisfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error resolving ref")
	}

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return err
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		// TODO add explicit SetArbitraryMetadata grant to CS3 api, tracked in https://github.com/cs3org/cs3apis/issues/91
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	nodePath := n.lu.toInternalPath(n.ID)
	for k, v := range md.Metadata {
		// TODO set etag as temporary etag tmpEtagAttr
		attrName := metadataPrefix + k
		if err = xattr.Set(nodePath, attrName, []byte(v)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set metadata attribute "+attrName+" to "+k)
		}
	}
	return
}

func (fs *ocisfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error resolving ref")
	}

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return err
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		// TODO use  SetArbitraryMetadata grant to CS3 api, tracked in https://github.com/cs3org/cs3apis/issues/91
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	nodePath := n.lu.toInternalPath(n.ID)
	for i := range keys {
		attrName := metadataPrefix + keys[i]
		if err = xattr.Remove(nodePath, attrName); err != nil {
			// a non-existing attribute will return an error, which we can ignore
			// (using string compare because the error type is syscall.Errno and not wrapped/recognizable)
			if e, ok := err.(*xattr.Error); !ok || e.Err.Error() != "no data available" {
				appctx.GetLogger(ctx).Error().Err(err).
					Interface("node", n).
					Str("key", keys[i]).
					Msg("could not unset metadata")
			} else {
				err = nil
			}
		}
	}
	return
}
