// Copyright 2018-2026 CERN
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

package eos

import (
	"context"
	"io"
	"path"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/pkg/errors"
)

func (fs *Eosfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	var auth eosclient.Authorization
	var fn string
	var err error

	md, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	fn = fs.wrap(ctx, md.Path)

	if md.PermissionSet.ListFileVersions {
		versionFolder := eosclient.GetVersionFolder(fn)
		auth, err = fs.getOwnerAuth(ctx, versionFolder)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to list revisions")
	}

	eosRevisions, err := fs.c.ListVersions(ctx, auth, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error listing versions")
	}
	revisions := []*provider.FileVersion{}
	for _, eosRev := range eosRevisions {
		if rev, err := fs.convertToRevision(ctx, eosRev); err == nil {
			revisions = append(revisions, rev)
		}
	}
	return revisions, nil
}

func (fs *Eosfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	var auth eosclient.Authorization
	var fn string
	var err error

	md, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	fn = fs.wrap(ctx, md.Path)

	if md.PermissionSet.InitiateFileDownload {
		versionFolder := eosclient.GetVersionFolder(fn)

		auth, err = fs.getOwnerAuth(ctx, versionFolder)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to download revisions")
	}

	return fs.c.ReadVersion(ctx, auth, fn, revisionKey)
}

func (fs *Eosfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	log := appctx.GetLogger(ctx)
	var auth eosclient.Authorization
	var fn string
	var err error

	md, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return err
	}
	fn = fs.wrap(ctx, md.Path)

	if md.PermissionSet.RestoreFileVersion {
		auth, err = fs.getOwnerAuth(ctx, fn)
		if err != nil {
			return err
		}
	} else {
		return errtypes.PermissionDenied("eosfs: user doesn't have permissions to restore revisions")
	}

	log.Debug().Any("auth", auth).Any("file", fn).Any("revision", revisionKey).Msg("eosfs RestoreRevision")
	return fs.c.RollbackToVersion(ctx, auth, fn, revisionKey)
}

func (fs *Eosfs) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.FileVersion, error) {
	md, err := fs.convertToResourceInfo(ctx, eosFileInfo)
	if err != nil {
		return nil, err
	}
	revision := &provider.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
		Etag:  md.Etag,
	}
	return revision, nil
}
