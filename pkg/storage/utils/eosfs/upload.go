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
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/eosclient"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/eosfs/upload"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, uff storage.UploadFinishedFunc) (provider.ResourceInfo, error) {
	upload, err := fs.GetUpload(ctx, ref.Path)
	if err != nil {
		return provider.ResourceInfo{}, err
	}
	info, err := upload.GetInfo(ctx)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	// We need the auth corresponding to the parent directory
	// as the file might not exist at the moment and we also
	// want to create files as the share owner in case of shares.
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: info.Storage["UID"],
			GID: info.Storage["GID"],
		},
	}

	if err := fs.c.Write(ctx, auth, info.Storage["StoragePath"], r); err != nil {
		return provider.ResourceInfo{}, err
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, info.Storage["StoragePath"])
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	ri, err := fs.convertToResourceInfo(ctx, eosFileInfo, info.Storage["SpaceRoot"], false)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	u, _ := ctxpkg.ContextGetUser(ctx)

	uploadRef := &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: info.MetaData["providerID"],
			SpaceId:   info.Storage["SpaceRoot"],
			OpaqueId:  info.Storage["SpaceRoot"],
		},
		Path: utils.MakeRelativePath(info.Storage["SpacePath"]),
	}
	uff(ri.Owner, u.Id, uploadRef) // call back to let them know the upload has finished

	return *ri, nil
}

func (fs *eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	space, err := fs.resolveSpace(ctx, ref)
	if err != nil {
		return nil, err
	}
	resPath, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	spacePath := strings.TrimPrefix(resPath, space.RootInfo.Path)
	fid, err := strconv.ParseUint(ref.GetResourceId().GetOpaqueId(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("error converting string to int for eos fileid: %s", ref.GetResourceId().GetOpaqueId())
	}

	rootAuth, err := fs.getRootAuth(ctx)
	if err != nil {
		return nil, err
	}
	parentInfo, err := fs.c.GetFileInfoByInode(ctx, rootAuth, fid)
	if err != nil {
		return nil, err
	}

	lockID, _ := ctxpkg.ContextGetLockID(ctx)

	info := tusd.FileInfo{
		ID: uuid.New().String(),
		MetaData: tusd.MetaData{
			"filename": filepath.Base(resPath),
			"dir":      filepath.Dir(resPath),
			"lockid":   lockID,
		},
		Size: uploadLength,
		Storage: map[string]string{
			"UID":                 strconv.FormatUint(parentInfo.UID, 10),
			"GID":                 strconv.FormatUint(parentInfo.GID, 10),
			"StoragePath":         resPath,
			"SpacePath":           spacePath,
			"SpaceRoot":           space.Id.OpaqueId,
			"SpaceOwnerOrManager": space.Owner.GetId().GetOpaqueId(),
		},
	}

	if metadata != nil {
		info.MetaData["providerID"] = metadata["providerID"]
	}

	upload, err := fs.NewUpload(ctx, info)
	if err != nil {
		return nil, err
	}

	info, _ = upload.GetInfo(ctx)

	return map[string]string{
		"simple": info.ID,
		"tus":    info.ID,
	}, nil
}

// To implement the core tus.io protocol as specified in https://tus.io/protocols/resumable-upload.html#core-protocol
// - the storage needs to implement NewUpload and GetUpload
// - the upload needs to implement the tusd.Upload interface: WriteChunk, GetInfo, GetReader and FinishUpload

// NewUpload returns a new tus Upload instance
func (fs *eosfs) NewUpload(ctx context.Context, info tusd.FileInfo) (tusd.Upload, error) {
	return upload.New(ctx, info, fs.conf.CacheDirectory, fs.c)
}

// GetUpload returns the Upload for the given upload id
func (fs *eosfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	return upload.Get(ctx, id, fs.conf.CacheDirectory, fs.c)
}

// AsTerminatableUpload returns a TerminatableUpload
// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// the storage needs to implement AsTerminatableUpload
func (fs *eosfs) AsTerminatableUpload(up tusd.Upload) tusd.TerminatableUpload {
	return up.(*upload.Upload)
}

// AsLengthDeclarableUpload returns a LengthDeclarableUpload
// To implement the creation-defer-length extension as specified in https://tus.io/protocols/resumable-upload.html#creation
// the storage needs to implement AsLengthDeclarableUpload
func (fs *eosfs) AsLengthDeclarableUpload(up tusd.Upload) tusd.LengthDeclarableUpload {
	return up.(*upload.Upload)
}

// AsConcatableUpload returns a ConcatableUpload
// To implement the concatenation extension as specified in https://tus.io/protocols/resumable-upload.html#concatenation
// the storage needs to implement AsConcatableUpload
func (fs *eosfs) AsConcatableUpload(up tusd.Upload) tusd.ConcatableUpload {
	return up.(*upload.Upload)
}

// UseIn tells the tus upload middleware which extensions it supports.
func (fs *eosfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}
