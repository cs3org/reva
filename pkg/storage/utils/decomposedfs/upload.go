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
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx/manager/tus"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
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
// FIXME metadata is actually used to carry all kinds of headers
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

	tusMetadata := tusd.MetaData{}

	// checksum is sent as tus Upload-Checksum header and should not magically become a metadata property
	if checksum, ok := headers["checksum"]; ok {
		parts := strings.SplitN(checksum, " ", 2)
		if len(parts) != 2 {
			return nil, errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
		}
		switch parts[0] {
		case "sha1", "md5", "adler32":
			tusMetadata[tus.CS3Prefix+"checksum"] = checksum
		default:
			return nil, errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
		}
	}

	// if mtime has been set via tus metadata, expose it as tus metadata
	if ocmtime, ok := headers["mtime"]; ok {
		if ocmtime != "null" {
			tusMetadata[tus.TusPrefix+"mtime"] = ocmtime
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

	usr := ctxpkg.ContextMustGetUser(ctx)

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
		info.MetaData[tus.CS3Prefix+"lockid"] = lockID
	}
	info.MetaData[tus.CS3Prefix+"dir"] = filepath.Dir(relative)

	// rewrite filename for old chunking v1
	if chunking.IsChunked(n.Name) {
		info.MetaData[tus.CS3Prefix+"chunk"] = n.Name
		bi, err := chunking.GetChunkBLOBInfo(n.Name)
		if err != nil {
			return nil, err
		}
		n.Name = bi.Path
	}

	info.MetaData[tus.CS3Prefix+"filename"] = n.Name
	info.MetaData[tus.CS3Prefix+"SpaceRoot"] = n.SpaceRoot.ID
	info.MetaData[tus.CS3Prefix+"SpaceOwnerOrManager"] = n.SpaceOwnerOrManager(ctx).GetOpaqueId()
	info.MetaData[tus.CS3Prefix+"providerID"] = headers["providerID"]

	info.MetaData[tus.CS3Prefix+"RevisionTime"] = time.Now().UTC().Format(time.RFC3339Nano)
	info.MetaData[tus.CS3Prefix+"NodeId"] = n.ID
	info.MetaData[tus.CS3Prefix+"NodeParentId"] = n.ParentID

	info.MetaData[tus.CS3Prefix+"ExecutantIdp"] = usr.Id.Idp
	info.MetaData[tus.CS3Prefix+"ExecutantId"] = usr.Id.OpaqueId
	info.MetaData[tus.CS3Prefix+"ExecutantType"] = utils.UserTypeToString(usr.Id.Type)
	info.MetaData[tus.CS3Prefix+"ExecutantUserName"] = usr.Username

	info.MetaData[tus.CS3Prefix+"LogLevel"] = sublog.GetLevel().String()

	// expires has been set by the storageprovider, do not expose as metadata. It is sent as a tus Upload-Expires header
	if expiration, ok := headers["expires"]; ok {
		if expiration != "null" { // TODO this is set by the storageprovider ... it cannot be set by cliensts, so it can never be the string 'null' ... or can it???
			info.MetaData[tus.CS3Prefix+"expires"] = expiration
		}
	}
	// only check preconditions if they are not empty
	// do not expose as metadata
	if headers["if-match"] != "" {
		info.MetaData[tus.CS3Prefix+"if-match"] = headers["if-match"] // TODO drop?
	}
	if headers["if-none-match"] != "" {
		info.MetaData[tus.CS3Prefix+"if-none-match"] = headers["if-none-match"]
	}
	if headers["if-unmodified-since"] != "" {
		info.MetaData[tus.CS3Prefix+"if-unmodified-since"] = headers["if-unmodified-since"]
	}

	if info.MetaData[tus.CS3Prefix+"if-none-match"] == "*" && n.Exists {
		return nil, errtypes.Aborted(fmt.Sprintf("parent %s already has a child %s", n.ID, n.Name))
	}

	// create the upload
	upload, err := fs.tusDataStore.NewUpload(ctx, info)
	if err != nil {
		return nil, err
	}

	info, _ = upload.GetInfo(ctx)

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

	// put lockID from upload back into context
	if info.MetaData[tus.CS3Prefix+"lockid"] != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, info.MetaData[tus.CS3Prefix+"lockid"])
	}

	log := appctx.GetLogger(ctx)

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
			return errtypes.InternalError(fmt.Sprintf("mismatching upload length. expected %d, could only copy %d", info.Size, bytesCopied))
		}
	}

	// compare if they match the sent checksum
	// TODO the tus checksum extension would do this on every chunk, but I currently don't see an easy way to pass in the requested checksum. for now we do it in FinishUpload which is also called for chunked uploads
	if info.MetaData[tus.CS3Prefix+"checksum"] != "" {
		var err error
		parts := strings.SplitN(info.MetaData[tus.CS3Prefix+"checksum"], " ", 2)
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
					log.Error().Err(terr).Msg("failed to terminate upload")
				}
			}
			return err
		}
	}

	// update checksums
	attrs := node.Attributes{
		prefixes.ChecksumPrefix + "sha1":    sha1h.Sum(nil),
		prefixes.ChecksumPrefix + "md5":     md5h.Sum(nil),
		prefixes.ChecksumPrefix + "adler32": adler32h.Sum(nil),
	}

	n, err := upload.AddRevisionToNode(ctx, fs.lu, info, attrs)
	if err != nil {
		upload.Cleanup(ctx, fs.lu, n, info, true)
		if tup, ok := up.(tusd.TerminatableUpload); ok {
			terr := tup.Terminate(ctx)
			if terr != nil {
				log.Error().Err(terr).Msg("failed to terminate upload")
			}
		}
		return err
	}

	if fs.stream != nil {
		user := &userpb.User{
			Id: &userpb.UserId{
				Type:     userpb.UserType(userpb.UserType_value[info.MetaData[tus.CS3Prefix+"ExecutantType"]]),
				Idp:      info.MetaData[tus.CS3Prefix+"ExecutantIdp"],
				OpaqueId: info.MetaData[tus.CS3Prefix+"ExecutantId"],
			},
			Username: info.MetaData[tus.CS3Prefix+"ExecutantUserName"],
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
			Filename:      info.MetaData[tus.CS3Prefix+"filename"],
			Filesize:      uint64(info.Size),
		}); err != nil {
			return err
		}
	}

	sizeDiff := info.Size - n.Blobsize
	if !fs.o.AsyncFileUploads {
		// handle postprocessing synchronously
		err = upload.Finalize(ctx, fs.blobstore, info, n) // moving or copying the blob only reads the blobid, no need to change the revision nodes nodeid
		upload.Cleanup(ctx, fs.lu, n, info, err != nil)
		if tup, ok := up.(tusd.TerminatableUpload); ok {
			terr := tup.Terminate(ctx)
			if terr != nil {
				log.Error().Err(terr).Msg("failed to terminate upload")
			}
		}
		if err != nil {
			log.Error().Err(err).Msg("failed to upload")
			return err
		}
		sizeDiff, err = upload.SetNodeToRevision(ctx, fs.lu, n, info.MetaData[tus.CS3Prefix+"RevisionTime"])
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

	uploadInfo, _ := up.GetInfo(ctx)

	p := uploadInfo.MetaData[tus.CS3Prefix+"chunk"]
	if chunking.IsChunked(p) { // check chunking v1
		var assembledFile string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(p, req.Body)
		if err != nil {
			return provider.ResourceInfo{}, err
		}
		if p == "" {
			return provider.ResourceInfo{}, errtypes.PartialContent(req.Ref.String())
		}
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
		bytesWritten, err := up.WriteChunk(ctx, 0, req.Body)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error writing to binary file")
		}
		if uploadInfo.SizeIsDeferred {
			// update the size
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

	n, err := upload.ReadNode(ctx, fs.lu, uploadInfo)
	if err != nil {
		return provider.ResourceInfo{}, err
	}

	if uff != nil {
		// TODO search needs to index the full path, so we return a reference relative to the space root.
		// but then the search has to walk the path. it might be more efficient if search called GetPath itself ... or we send the path as additional metadata in the event
		uploadRef := &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: uploadInfo.MetaData[tus.CS3Prefix+"providerID"],
				SpaceId:   n.SpaceID,
				OpaqueId:  n.SpaceID,
			},
			Path: utils.MakeRelativePath(filepath.Join(uploadInfo.MetaData[tus.CS3Prefix+"dir"], uploadInfo.MetaData[tus.CS3Prefix+"filename"])),
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
			StorageId: uploadInfo.MetaData[tus.CS3Prefix+"providerID"],
			SpaceId:   n.SpaceID,
			OpaqueId:  n.ID,
		},
		Etag: etag,
	}

	if mtime, err := utils.MTimeToTS(uploadInfo.MetaData[tus.TusPrefix+"mtime"]); err == nil {
		ri.Mtime = &mtime
	}

	return ri, nil
}

// FIXME all the below functions should needs a dedicated package ... the tusd datastore interface has no way of listing uploads, so we need to extend them

// ListUploads returns a list of all incomplete uploads
func (fs *Decomposedfs) ListUploads() ([]tusd.FileInfo, error) {
	return fs.uploadInfos(context.Background())
}

// PurgeExpiredUploads scans the fs for expired downloads and removes any leftovers
func (fs *Decomposedfs) PurgeExpiredUploads(purgedChan chan<- tusd.FileInfo) error {
	infos, err := fs.uploadInfos(context.Background())
	if err != nil {
		return err
	}

	for _, info := range infos {
		expires, err := strconv.Atoi(info.MetaData[tus.CS3Prefix+"expires"])
		if err != nil {
			continue
		}
		if int64(expires) < time.Now().Unix() {
			purgedChan <- info
			err = os.Remove(info.Storage["BinPath"]) // FIXME
			if err != nil {
				return err
			}
			err = os.Remove(filepath.Join(fs.o.Root, "uploads", info.ID+".info"))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (fs *Decomposedfs) uploadInfos(ctx context.Context) ([]tusd.FileInfo, error) {
	infos := []tusd.FileInfo{}
	infoFiles, err := filepath.Glob(filepath.Join(fs.o.Root, "uploads", "*.info")) // FIXME
	if err != nil {
		return nil, err
	}

	for _, info := range infoFiles {
		match := _idRegexp.FindStringSubmatch(info)
		if match == nil || len(match) < 2 {
			continue
		}
		up, err := fs.tusDataStore.GetUpload(ctx, match[1])
		if err != nil {
			return nil, err
		}
		info, err := up.GetInfo(context.Background())
		if err != nil {
			return nil, err
		}

		infos = append(infos, info)
	}
	return infos, nil
}
