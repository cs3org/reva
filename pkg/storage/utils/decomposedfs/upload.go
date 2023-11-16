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
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	tusd "github.com/tus/tusd/pkg/handler"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/upload"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/pkg/errors"
)

var _idRegexp = regexp.MustCompile(".*/([^/]+).info")

// InitiateUpload returns upload ids corresponding to different protocols it supports
// It creates a node for new files to persist the fileid for the new child.
// TODO read optional content for small files in this request
// TODO InitiateUpload (and Upload) needs a way to receive the expected checksum. Maybe in metadata as 'checksum' => 'sha1 aeosvp45w5xaeoe' = lowercase, space separated?
// TODO needs a way to handle unknown filesize, currently uses the context
// FIXME headers is actually used to carry all kinds of headers
func (fs *Decomposedfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, headers map[string]string) (map[string]string, error) {

	n, err := fs.lu.NodeFromResource(ctx, ref)
	switch err.(type) {
	case nil:
		// ok
	case errtypes.IsNotFound:
		return nil, errtypes.PreconditionFailed(err.Error())
	default:
		return nil, err
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", n.SpaceID).Str("nodeid", n.ID).Int64("uploadLength", uploadLength).Interface("headers", headers).Logger()

	// permissions are checked in NewUpload below

	relative, err := fs.lu.Path(ctx, n, node.NoCheck)
	if err != nil {
		return nil, err
	}

	usr := ctxpkg.ContextMustGetUser(ctx)
	cs3Metadata := upload.Metadata{
		Filename:            n.Name,
		SpaceRoot:           n.SpaceRoot.ID,
		SpaceOwnerOrManager: n.SpaceOwnerOrManager(ctx).GetOpaqueId(),
		ProviderID:          headers["providerID"],
		RevisionTime:        time.Now().UTC().Format(time.RFC3339Nano),
		NodeID:              n.ID,
		NodeParentID:        n.ParentID,
		ExecutantIdp:        usr.Id.Idp,
		ExecutantID:         usr.Id.OpaqueId,
		ExecutantType:       utils.UserTypeToString(usr.Id.Type),
		ExecutantUserName:   usr.Username,
		LogLevel:            sublog.GetLevel().String(),
	}

	tusMetadata := tusd.MetaData{}

	// checksum is sent as tus Upload-Checksum header and should not magically become a metadata property
	if checksum, ok := headers["checksum"]; ok {
		parts := strings.SplitN(checksum, " ", 2)
		if len(parts) != 2 {
			return nil, errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
		}
		switch parts[0] {
		case "sha1", "md5", "adler32":
			cs3Metadata.Checksum = checksum
		default:
			return nil, errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
		}
	}

	// if mtime has been set via tus metadata, expose it as tus metadata
	if ocmtime, ok := headers["mtime"]; ok {
		if ocmtime != "null" {
			tusMetadata["mtime"] = ocmtime
		}
	}

	_, err = node.CheckQuota(ctx, n.SpaceRoot, n.Exists, uint64(n.Blobsize), uint64(uploadLength))
	if err != nil {
		return nil, err
	}

	// check permissions
	var (
		checkNode *node.Node
		path      string
	)
	if n.Exists {
		// check permissions of file to be overwritten
		checkNode = n
		path, _ = storagespace.FormatReference(&provider.Reference{ResourceId: &provider.ResourceId{
			SpaceId:  checkNode.SpaceID,
			OpaqueId: checkNode.ID,
		}})
		previousRevisionTime, err := n.GetCurrentRevision(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "Decomposedfs: error current revision of "+n.ID) // TODO this will be the case for all existing files
			// fallback to mtime?
		}
		cs3Metadata.PreviousRevisionTime = previousRevisionTime
	} else {
		// check permissions of parent
		parent, perr := n.Parent(ctx)
		if perr != nil {
			return nil, errors.Wrap(perr, "Decomposedfs: error getting parent "+n.ParentID)
		}
		checkNode = parent
		path, _ = storagespace.FormatReference(&provider.Reference{ResourceId: &provider.ResourceId{
			SpaceId:  checkNode.SpaceID,
			OpaqueId: checkNode.ID,
		}, Path: n.Name})
	}
	rp, err := fs.p.AssemblePermissions(ctx, checkNode) // context does not have a user?
	switch {
	case err != nil:
		return nil, err
	case !rp.InitiateFileUpload:
		return nil, errtypes.PermissionDenied(path)
	}

	// are we trying to overwrite a folder with a file?
	if n.Exists && n.IsDir(ctx) {
		return nil, errtypes.PreconditionFailed("resource is not a file")
	}

	// check lock
	// FIXME we cannot check the lock of a new file, because it would have to use the name ...
	if err := n.CheckLock(ctx); err != nil {
		return nil, err
	}

	// treat 0 length uploads as deferred
	sizeIsDeferred := false
	if uploadLength == 0 {
		sizeIsDeferred = true
	}

	info := tusd.FileInfo{
		MetaData:       tusMetadata,
		Size:           uploadLength,
		SizeIsDeferred: sizeIsDeferred,
	}
	if lockID, ok := ctxpkg.ContextGetLockID(ctx); ok {
		cs3Metadata.LockID = lockID
	}
	cs3Metadata.Dir = filepath.Dir(relative)

	// rewrite filename for old chunking v1
	if chunking.IsChunked(n.Name) {
		cs3Metadata.Chunk = n.Name
		bi, err := chunking.GetChunkBLOBInfo(n.Name)
		if err != nil {
			return nil, err
		}
		n.Name = bi.Path
	}

	// TODO at this point we have no way to figure out the output or mode of the logger. we need that to reinitialize a logger in PreFinishResponseCallback
	// or better create a config option for the log level during PreFinishResponseCallback? might be easier for now

	// expires has been set by the storageprovider, do not expose as metadata. It is sent as a tus Upload-Expires header
	if expiration, ok := headers["expires"]; ok {
		if expiration != "null" { // TODO this is set by the storageprovider ... it cannot be set by cliensts, so it can never be the string 'null' ... or can it???
			cs3Metadata.Expires = expiration
		}
	}
	// only check preconditions if they are not empty
	// do not expose as metadata
	if headers["if-match"] != "" {
		cs3Metadata.HeaderIfMatch = headers["if-match"] // TODO drop?
	}
	if headers["if-none-match"] != "" {
		cs3Metadata.HeaderIfNoneMatch = headers["if-none-match"]
	}
	if headers["if-unmodified-since"] != "" {
		cs3Metadata.HeaderIfUnmodifiedSince = headers["if-unmodified-since"]
	}

	if cs3Metadata.HeaderIfNoneMatch == "*" && n.Exists {
		return nil, errtypes.Aborted(fmt.Sprintf("parent %s already has a child %s", n.ID, n.Name))
	}

	// create the upload
	u, err := fs.tusDataStore.NewUpload(ctx, info)
	if err != nil {
		return nil, err
	}

	info, err = u.GetInfo(ctx)
	if err != nil {
		return nil, err
	}

	cs3Metadata.ID = info.ID

	// keep track of upload
	err = upload.WriteMetadata(ctx, fs.lu, info.ID, cs3Metadata)
	if err != nil {
		return nil, err
	}

	sublog.Debug().Interface("info", info).Msg("Decomposedfs: initiated upload")

	return map[string]string{
		"simple": info.ID,
		"tus":    info.ID,
	}, nil
}

// GetDataStore returns the initialized Datastore
func (fs *Decomposedfs) GetDataStore() tusd.DataStore {
	return fs.tusDataStore
}

// PreFinishResponseCallback is called by the tus datatx, after all bytes have been transferred
func (fs *Decomposedfs) PreFinishResponseCallback(hook tusd.HookEvent) error {
	ctx := context.TODO()
	appctx.GetLogger(ctx).Debug().Interface("hook", hook).Msg("got PreFinishResponseCallback")
	ctx, span := tracer.Start(ctx, "PreFinishResponseCallback")
	defer span.End()

	info := hook.Upload
	up, err := fs.tusDataStore.GetUpload(ctx, info.ID)
	if err != nil {
		return err
	}

	uploadMetadata, err := upload.ReadMetadata(ctx, fs.lu, info.ID)
	if err != nil {
		return err
	}

	// put lockID from upload back into context
	if uploadMetadata.LockID != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, uploadMetadata.LockID)
	}

	// restore logger from file info
	log, err := logger.FromConfig(&logger.LogConf{
		Output: "stdout",
		Mode:   "json",
		Level:  uploadMetadata.LogLevel,
	})
	if err != nil {
		return err
	}

	ctx = appctx.WithLogger(ctx, log)

	// calculate the checksum of the written bytes
	// they will all be written to the metadata later, so we cannot omit any of them
	// TODO only calculate the checksum in sync that was requested to match, the rest could be async ... but the tests currently expect all to be present
	// TODO the hashes all implement BinaryMarshaler so we could try to persist the state for resumable upload. we would neet do keep track of the copied bytes ...

	sha1h := sha1.New()
	md5h := md5.New()
	adler32h := adler32.New()
	{
		_, subspan := tracer.Start(ctx, "GetReader")
		reader, err := up.GetReader(ctx)
		subspan.End()
		if err != nil {
			// we can continue if no oc checksum header is set
			log.Info().Err(err).Interface("info", info).Msg("error getting Reader from upload")
		}
		if readCloser, ok := reader.(io.ReadCloser); ok {
			defer readCloser.Close()
		}

		r1 := io.TeeReader(reader, sha1h)
		r2 := io.TeeReader(r1, md5h)

		_, subspan = tracer.Start(ctx, "io.Copy")
		bytesCopied, err := io.Copy(adler32h, r2)
		subspan.End()
		if err != nil {
			log.Info().Err(err).Msg("error copying checksums")
		}
		if bytesCopied != info.Size {
			msg := fmt.Sprintf("mismatching upload length. expected %d, could only copy %d", info.Size, bytesCopied)
			log.Error().Interface("info", info).Msg(msg)
			return errtypes.InternalError(msg)
		}
	}

	// compare if they match the sent checksum
	// TODO the tus checksum extension would do this on every chunk, but I currently don't see an easy way to pass in the requested checksum. for now we do it in FinishUpload which is also called for chunked uploads
	if uploadMetadata.Checksum != "" {
		var err error
		parts := strings.SplitN(uploadMetadata.Checksum, " ", 2)
		if len(parts) != 2 {
			return errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
		}
		switch parts[0] {
		case "sha1":
			err = checkHash(parts[1], sha1h)
		case "md5":
			err = checkHash(parts[1], md5h)
		case "adler32":
			err = checkHash(parts[1], adler32h)
		default:
			err = errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
		}
		if err != nil {
			if tup, ok := up.(tusd.TerminatableUpload); ok {
				terr := tup.Terminate(ctx)
				if terr != nil {
					log.Error().Err(terr).Interface("info", info).Msg("failed to terminate upload")
				}
			}
			return err
		}
	}

	// update checksums
	uploadMetadata.ChecksumSHA1 = sha1h.Sum(nil)
	uploadMetadata.ChecksumMD5 = md5h.Sum(nil)
	uploadMetadata.ChecksumADLER32 = adler32h.Sum(nil)
	// set mtime for revision
	if info.MetaData["mtime"] == "" {
		uploadMetadata.MTime = uploadMetadata.RevisionTime
	} else {
		// overwrite mtime if requested
		mtime, err := utils.MTimeToTime(info.MetaData["mtime"])
		if err != nil {
			return err
		}
		uploadMetadata.MTime = mtime.UTC().Format(time.RFC3339Nano)
	}

	uploadMetadata, n, err := upload.UpdateMetadata(ctx, fs.lu, info.ID, info.Size, uploadMetadata)
	if err != nil {
		upload.Cleanup(ctx, fs.lu, n, info.ID, uploadMetadata.RevisionTime, uploadMetadata.PreviousRevisionTime, true)
		if tup, ok := up.(tusd.TerminatableUpload); ok {
			terr := tup.Terminate(ctx)
			if terr != nil {
				log.Error().Err(terr).Interface("info", info).Msg("failed to terminate upload")
			}
		}
		return err
	}

	if fs.stream != nil {
		user := &userpb.User{
			Id: &userpb.UserId{
				Type:     userpb.UserType(userpb.UserType_value[uploadMetadata.ExecutantType]),
				Idp:      uploadMetadata.ExecutantIdp,
				OpaqueId: uploadMetadata.ExecutantID,
			},
			Username: uploadMetadata.ExecutantUserName,
		}
		s, err := fs.downloadURL(ctx, info.ID)
		if err != nil {
			return err
		}

		if err := events.Publish(ctx, fs.stream, events.BytesReceived{
			UploadID:      info.ID,
			URL:           s,
			SpaceOwner:    n.SpaceOwnerOrManager(ctx),
			ExecutingUser: user,
			ResourceID:    &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID},
			Filename:      uploadMetadata.Filename, // TODO what and when do we publish chunking v2 names? Currently, this uses the chunk name.
			Filesize:      uint64(info.Size),
		}); err != nil {
			return err
		}
	}

	sizeDiff := info.Size - n.Blobsize
	if !fs.o.AsyncFileUploads {
		// handle postprocessing synchronously
		err = upload.Finalize(ctx, fs.blobstore, uploadMetadata.RevisionTime, info, n, uploadMetadata.BlobID) // moving or copying the blob only reads the blobid, no need to change the revision nodes nodeid
		upload.Cleanup(ctx, fs.lu, n, info.ID, uploadMetadata.RevisionTime, uploadMetadata.PreviousRevisionTime, err != nil)
		if tup, ok := up.(tusd.TerminatableUpload); ok {
			terr := tup.Terminate(ctx)
			if terr != nil {
				log.Error().Err(terr).Interface("info", info).Msg("failed to terminate upload")
			}
		}
		if err != nil {
			log.Error().Err(err).Msg("failed to upload")
			return err
		}
		sizeDiff, err = upload.SetNodeToUpload(ctx, fs.lu, n, uploadMetadata)
		if err != nil {
			log.Error().Err(err).Msg("failed update Node to revision")
			return err
		}
	}

	return fs.tp.Propagate(ctx, n, sizeDiff)
}

// URL returns a url to download an upload
func (fs *Decomposedfs) downloadURL(_ context.Context, id string) (string, error) {
	type transferClaims struct {
		jwt.StandardClaims
		Target string `json:"target"`
	}

	u, err := url.JoinPath(fs.o.Tokens.DownloadEndpoint, "tus/", id)
	if err != nil {
		return "", errors.Wrapf(err, "error joinging URL path")
	}
	ttl := time.Duration(fs.o.Tokens.TransferExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		Target: u,
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(fs.o.Tokens.TransferSharedSecret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return url.JoinPath(fs.o.Tokens.DataGatewayEndpoint, tkn)
}

func checkHash(expected string, h hash.Hash) error {
	if expected != hex.EncodeToString(h.Sum(nil)) {
		return errtypes.ChecksumMismatch(fmt.Sprintf("invalid checksum: expected %s got %x", expected, h.Sum(nil)))
	}
	return nil
}

// Upload uploads data to the given resource
// is used by the simple datatx, after an InitiateUpload call
// TODO Upload (and InitiateUpload) needs a way to receive the expected checksum.
// Maybe in metadata as 'checksum' => 'sha1 aeosvp45w5xaeoe' = lowercase, space separated?
func (fs *Decomposedfs) Upload(ctx context.Context, req storage.UploadRequest, uff storage.UploadFinishedFunc) (provider.ResourceInfo, error) {
	up, err := fs.tusDataStore.GetUpload(ctx, req.Ref.GetPath())
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error retrieving upload")
	}

	uploadInfo, err := up.GetInfo(ctx)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error retrieving upload info")
	}

	uploadMetadata, err := upload.ReadMetadata(ctx, fs.lu, uploadInfo.ID)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error retrieving upload metadata")
	}

	if chunking.IsChunked(uploadMetadata.Chunk) { // check chunking v1, TODO, actually there is a 'OC-Chunked: 1' header, at least when the testsuite uses chunking v1
		var assembledFile, p string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(uploadMetadata.Chunk, req.Body)
		if err != nil {
			return provider.ResourceInfo{}, err
		}
		if p == "" {
			return provider.ResourceInfo{}, errtypes.PartialContent(req.Ref.String())
		}
		uploadMetadata.Filename = p
		uploadInfo.MetaData["filename"] = p
		fd, err := os.Open(assembledFile)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)

		chunkStat, err := fd.Stat()
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: could not stat assembledFile for legacy chunking")
		}

		// fake a new upload with the correct size
		newInfo := tusd.FileInfo{
			Size:     chunkStat.Size(),
			MetaData: uploadInfo.MetaData,
		}
		nup, err := fs.tusDataStore.NewUpload(ctx, newInfo)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: could not create new tus upload for legacy chunking")
		}
		newInfo, err = nup.GetInfo(ctx)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: could not get info from upload")
		}
		uploadMetadata.ID = newInfo.ID
		uploadMetadata.BlobSize = newInfo.Size
		err = upload.WriteMetadata(ctx, fs.lu, newInfo.ID, uploadMetadata)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error writing upload metadata for legacy chunking")
		}

		_, err = nup.WriteChunk(ctx, 0, fd)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error writing to binary file for legacy chunking")
		}
		// use new upload and info
		up = nup
		uploadInfo, err = up.GetInfo(ctx)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: could not get info for legacy chunking")
		}
	} else {
		// we need to call up.DeclareLength() before writing the chunk, but only if we actually got a length
		if req.Length > 0 {
			if ldx, ok := up.(tusd.LengthDeclarableUpload); ok {
				if err := ldx.DeclareLength(ctx, req.Length); err != nil {
					return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error declaring length")
				}
			}
		}
		bytesWritten, err := up.WriteChunk(ctx, 0, req.Body)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error writing to binary file")
		}
		uploadInfo.Offset += bytesWritten
		if uploadInfo.SizeIsDeferred {
			// update the size and offset
			uploadInfo.SizeIsDeferred = false
			uploadInfo.Size = bytesWritten
		}
	}

	// This finishes the tus upload
	if err := up.FinishUpload(ctx); err != nil {
		return provider.ResourceInfo{}, err
	}

	// we now need to handle to move/copy&delete to the target blobstore
	err = fs.PreFinishResponseCallback(tusd.HookEvent{Upload: uploadInfo})
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	n, err := upload.ReadNode(ctx, fs.lu, uploadMetadata)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	if uff != nil {
		// TODO search needs to index the full path, so we return a reference relative to the space root.
		// but then the search has to walk the path. it might be more efficient if search called GetPath itself ... or we send the path as additional metadata in the event
		uploadRef := &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: uploadMetadata.ProviderID,
				SpaceId:   n.SpaceID,
				OpaqueId:  n.SpaceID,
			},
			Path: utils.MakeRelativePath(filepath.Join(uploadMetadata.Dir, uploadMetadata.Filename)),
		}
		excutant, ok := ctxpkg.ContextGetUser(ctx)
		if !ok {
			return provider.ResourceInfo{}, errtypes.PreconditionFailed("error getting user from context")
		}

		uff(n.SpaceOwnerOrManager(ctx), excutant.Id, uploadRef)
	}

	mtime, err := n.GetMTime(ctx)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error getting mtime for '"+n.ID+"'")
	}
	etag, err := node.CalculateEtag(n, mtime)
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error calculating etag '"+n.ID+"'")
	}
	ri := provider.ResourceInfo{
		// fill with at least fileid, mtime and etag
		Id: &provider.ResourceId{
			StorageId: uploadMetadata.ProviderID,
			SpaceId:   n.SpaceID,
			OpaqueId:  n.ID,
		},
		Etag: etag,
	}

	if mtime, err := utils.MTimeToTS(uploadInfo.MetaData["mtime"]); err == nil {
		ri.Mtime = &mtime
	}

	return ri, nil
}

// FIXME all the below functions should needs a dedicated package ... the tusd datastore interface has no way of listing uploads, so we need to extend them

// GetUploadMetadata returns the metadata for the given upload id
func (fs *Decomposedfs) GetUploadMetadata(ctx context.Context, uploadID string) (storage.UploadMetadata, error) {
	return upload.ReadMetadata(ctx, fs.lu, uploadID)
}

// ListUploads returns a list of all incomplete uploads
func (fs *Decomposedfs) ListUploads() ([]storage.UploadMetadata, error) {
	return fs.uploadInfos(context.Background())
}

// PurgeExpiredUploads scans the fs for expired downloads and removes any leftovers
func (fs *Decomposedfs) PurgeExpiredUploads(purgedChan chan<- storage.UploadMetadata) error {
	metadata, err := fs.uploadInfos(context.Background())
	if err != nil {
		return err
	}

	for _, m := range metadata {
		uploadMetadata, err := upload.ReadMetadata(context.TODO(), fs.lu, m.GetID())
		if err != nil {
			continue
		}
		expires, err := strconv.Atoi(uploadMetadata.Expires)
		if err != nil {
			continue
		}
		if int64(expires) < time.Now().Unix() {
			purgedChan <- uploadMetadata
			up, err := fs.tusDataStore.GetUpload(context.Background(), m.GetID())
			if err != nil {
				return err
			}
			if tu, ok := up.(tusd.TerminatableUpload); ok {
				err = tu.Terminate(context.Background())
				if err != nil {
					return err
				}
			}
			err = os.Remove(fs.lu.UploadPath(m.GetID()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (fs *Decomposedfs) uploadInfos(ctx context.Context) ([]storage.UploadMetadata, error) {
	metadata := []storage.UploadMetadata{}
	metadataFiles, err := filepath.Glob(filepath.Join(fs.o.Root, "uploads", "*.mpk")) // FIXME
	if err != nil {
		return nil, err
	}

	for _, f := range metadataFiles {
		match := _idRegexp.FindStringSubmatch(f)
		if match == nil || len(match) < 2 {
			continue
		}
		up, err := fs.GetUploadMetadata(ctx, match[1])
		if err != nil {
			return nil, err
		}

		metadata = append(metadata, up)
	}
	return metadata, nil
}
