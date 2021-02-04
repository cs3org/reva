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

package decomposedfs

import (
	"context"
	"fmt"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// SetArbitraryMetadata sets the metadata on a resource
func (fs *Decomposedfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error resolving ref")
	}
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()

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

	nodePath := n.InternalPath()

	errs := []error{}
	// TODO should we really continue updating when an error occurs?
	if md.Metadata != nil {
		if val, ok := md.Metadata["mtime"]; ok {
			delete(md.Metadata, "mtime")
			err := n.SetMtime(ctx, val)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "could not set mtime"))
			}
		}
		// TODO(jfd) special handling for atime?
		// TODO(jfd) allow setting birth time (btime)?
		// TODO(jfd) any other metadata that is interesting? fileid?
		// TODO unset when file is updated
		// TODO unset when folder is updated or add timestamp to etag?
		if val, ok := md.Metadata["etag"]; ok {
			delete(md.Metadata, "etag")
			err := n.SetEtag(ctx, val)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "could not set etag"))
			}
		}
		if val, ok := md.Metadata[node.FavoriteKey]; ok {
			delete(md.Metadata, node.FavoriteKey)
			if u, ok := user.ContextGetUser(ctx); ok {
				if uid := u.GetId(); uid != nil {
					if err := n.SetFavorite(uid, val); err != nil {
						sublog.Error().Err(err).
							Interface("user", u).
							Msg("could not set favorite flag")
						errs = append(errs, errors.Wrap(err, "could not set favorite flag"))
					}
				} else {
					sublog.Error().Interface("user", u).Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				sublog.Error().Interface("user", u).Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
		}
	}
	for k, v := range md.Metadata {
		attrName := xattrs.MetadataPrefix + k
		if err = xattr.Set(nodePath, attrName, []byte(v)); err != nil {
			errs = append(errs, errors.Wrap(err, "Decomposedfs: could not set metadata attribute "+attrName+" to "+k))
		}
	}

	switch len(errs) {
	case 0:
		return fs.tp.Propagate(ctx, n)
	case 1:
		// TODO Propagate if anything changed
		return errs[0]
	default:
		// TODO Propagate if anything changed
		// TODO how to return multiple errors?
		return errors.New("multiple errors occurred, see log for details")
	}
}

// UnsetArbitraryMetadata unsets the metadata on the given resource
func (fs *Decomposedfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error resolving ref")
	}
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return err
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		// TODO use SetArbitraryMetadata grant to CS3 api, tracked in https://github.com/cs3org/cs3apis/issues/91
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	nodePath := n.InternalPath()
	errs := []error{}
	for _, k := range keys {
		switch k {
		case node.FavoriteKey:
			if u, ok := user.ContextGetUser(ctx); ok {
				// the favorite flag is specific to the user, so we need to incorporate the userid
				if uid := u.GetId(); uid != nil {
					fa := fmt.Sprintf("%s%s@%s", xattrs.FavPrefix, uid.GetOpaqueId(), uid.GetIdp())
					if err := xattr.Remove(nodePath, fa); err != nil {
						sublog.Error().Err(err).
							Interface("user", u).
							Str("key", fa).
							Msg("could not unset favorite flag")
						errs = append(errs, errors.Wrap(err, "could not unset favorite flag"))
					}
				} else {
					sublog.Error().
						Interface("user", u).
						Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				sublog.Error().
					Interface("user", u).
					Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
		default:
			if err = xattr.Remove(nodePath, xattrs.MetadataPrefix+k); err != nil {
				// a non-existing attribute will return an error, which we can ignore
				// (using string compare because the error type is syscall.Errno and not wrapped/recognizable)
				if e, ok := err.(*xattr.Error); !ok || !(e.Err.Error() == "no data available" ||
					// darwin
					e.Err.Error() == "attribute not found") {
					sublog.Error().Err(err).
						Str("key", k).
						Msg("could not unset metadata")
					errs = append(errs, errors.Wrap(err, "could not unset metadata"))
				}
			}
		}
	}
	switch len(errs) {
	case 0:
		return fs.tp.Propagate(ctx, n)
	case 1:
		// TODO Propagate if anything changed
		return errs[0]
	default:
		// TODO Propagate if anything changed
		// TODO how to return multiple errors?
		return errors.New("multiple errors occurred, see log for details")
	}
}
