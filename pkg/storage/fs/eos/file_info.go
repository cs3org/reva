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
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/pkg/errors"
)

func (fs *Eosfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error parsing fileid string")
	}

	auth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error getting file info by inode")
	}

	if perm := fs.permissionSet(ctx, eosFileInfo, nil); !perm.GetPath {
		return "", errtypes.PermissionDenied("eosfs: getting path for id not allowed")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
}

func (fs *Eosfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("eosfs: get md for ref:" + ref.String())

	if ref == nil {
		return nil, errtypes.BadRequest("No ref was given to GetMD")
	}

	// We use daemon for auth because we need access to the file in order to stat it
	// We cannot use the current user, because the file may be a shared file
	// and lightweight accounts don't have a uid
	auth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting daemon auth")
	}

	// First we check if it's a version (calling fs.resolve would fail, because
	// there is no path, and the opaque id is not an integer)
	if ref.ResourceId != nil && strings.Contains(ref.ResourceId.OpaqueId, "@") {
		parts := strings.Split(ref.ResourceId.OpaqueId, "@")
		opaqueID, version := parts[0], parts[1]

		fileId := &provider.ResourceId{
			StorageId: ref.ResourceId.StorageId,
			SpaceId:   ref.ResourceId.SpaceId,
			OpaqueId:  opaqueID,
		}
		filePath, err := fs.getPath(ctx, fileId)
		if err != nil {
			return nil, fmt.Errorf("error getting path for resource id: %s", opaqueID)
		}
		filePath = fs.wrap(ctx, filePath)

		versionFolder := eosclient.GetVersionFolder(filePath)
		versionPath := filepath.Join(versionFolder, version)
		eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, versionPath)
		if err != nil {
			return nil, fmt.Errorf("error getting file info by path: %s", versionPath)
		}

		return fs.convertToResourceInfo(ctx, eosFileInfo)
	}

	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errtypes.BadRequest("No ref was given to GetMD")
	}

	if ref.ResourceId != nil {
		fid, err := strconv.ParseUint(ref.ResourceId.OpaqueId, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting string to int for eos fileid: %s", ref.ResourceId.OpaqueId)
		}

		eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
		if err != nil {
			log.Error().Err(err).Str("fid", strconv.Itoa(int(fid))).Msg("Failed to get file info by inode")
			return nil, err
		}

		if ref.Path != "" {
			fn := filepath.Join(eosFileInfo.File, ref.Path)
			eosFileInfo, err = fs.c.GetFileInfoByPath(ctx, auth, fn)
			if err != nil {
				log.Error().Err(err).Str("path", fn).Msg("Failed to get file info by path")
				return nil, err
			}
		}
		return fs.convertToResourceInfo(ctx, eosFileInfo)
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, fn)
	if err != nil {
		log.Error().Err(err).Str("path", fn).Msg("Failed to get file info by path")
		return nil, err
	}

	return fs.convertToResourceInfo(ctx, eosFileInfo)
}

func (fs *Eosfs) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.ResourceInfo, error) {
	return fs.convert(ctx, eosFileInfo)
}

func (fs *Eosfs) getPath(ctx context.Context, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", fmt.Errorf("error converting string to int for eos fileid: %s", id.OpaqueId)
	}

	auth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error getting file info by inode")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
}
