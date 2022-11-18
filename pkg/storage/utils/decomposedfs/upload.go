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
	"encoding/json"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tusd "github.com/tus/tusd/pkg/handler"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var defaultFilePerm = os.FileMode(0664)

// Upload uploads data to the given resource
// TODO Upload (and InitiateUpload) needs a way to receive the expected checksum.
// Maybe in metadata as 'checksum' => 'sha1 aeosvp45w5xaeoe' = lowercase, space separated?
func (fs *Decomposedfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, uff storage.UploadFinishedFunc) (provider.ResourceInfo, error) {
	upload, err := fs.GetUpload(ctx, ref.GetPath())
	if err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error retrieving upload")
	}

	uploadInfo := upload.(*fileUpload)

	p := uploadInfo.info.Storage["NodeName"]
	if chunking.IsChunked(p) { // check chunking v1
		var assembledFile string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(p, r)
		if err != nil {
			return provider.ResourceInfo{}, err
		}
		if p == "" {
			if err = uploadInfo.Terminate(ctx); err != nil {
				return provider.ResourceInfo{}, errors.Wrap(err, "ocfs: error removing auxiliary files")
			}
			return provider.ResourceInfo{}, errtypes.PartialContent(ref.String())
		}
		uploadInfo.info.Storage["NodeName"] = p
		fd, err := os.Open(assembledFile)
		if err != nil {
			return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	if _, err := uploadInfo.WriteChunk(ctx, 0, r); err != nil {
		return provider.ResourceInfo{}, errors.Wrap(err, "Decomposedfs: error writing to binary file")
	}

	if err := uploadInfo.FinishUpload(ctx); err != nil {
		return provider.ResourceInfo{}, err
	}

	if uff != nil {
		info := uploadInfo.info
		uploadRef := &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: info.MetaData["providerID"],
				SpaceId:   info.Storage["SpaceRoot"],
				OpaqueId:  info.Storage["SpaceRoot"],
			},
			Path: utils.MakeRelativePath(filepath.Join(info.MetaData["dir"], info.MetaData["filename"])),
		}
		owner, ok := ctxpkg.ContextGetUser(uploadInfo.ctx)
		if !ok {
			return provider.ResourceInfo{}, errtypes.PreconditionFailed("error getting user from uploadinfo context")
		}
		spaceOwner := &userpb.UserId{
			OpaqueId: info.Storage["SpaceOwnerOrManager"],
		}
		uff(spaceOwner, owner.Id, uploadRef)
	}

	ri := provider.ResourceInfo{
		// fill with at least fileid, mtime and etag
		Id: &provider.ResourceId{
			StorageId: uploadInfo.info.MetaData["providerID"],
			SpaceId:   uploadInfo.info.Storage["SpaceRoot"],
			OpaqueId:  uploadInfo.info.Storage["NodeId"],
		},
		Etag: uploadInfo.info.MetaData["etag"],
	}

	if mtime, err := utils.MTimeToTS(uploadInfo.info.MetaData["mtime"]); err == nil {
		ri.Mtime = &mtime
	}

	return ri, nil
}

// InitiateUpload returns upload ids corresponding to different protocols it supports
// TODO read optional content for small files in this request
// TODO InitiateUpload (and Upload) needs a way to receive the expected checksum. Maybe in metadata as 'checksum' => 'sha1 aeosvp45w5xaeoe' = lowercase, space separated?
func (fs *Decomposedfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	log := appctx.GetLogger(ctx)

	n, err := fs.lu.NodeFromResource(ctx, ref)
	switch err.(type) {
	case nil:
		// ok
	case errtypes.IsNotFound:
		return nil, errtypes.PreconditionFailed(err.Error())
	default:
		return nil, err
	}

	// permissions are checked in NewUpload below

	relative, err := fs.lu.Path(ctx, n)
	if err != nil {
		return nil, err
	}

	lockID, _ := ctxpkg.ContextGetLockID(ctx)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(relative),
			"dir":      filepath.Dir(relative),
			"lockid":   lockID,
		},
		Size: uploadLength,
		Storage: map[string]string{
			"SpaceRoot":           n.SpaceRoot.ID,
			"SpaceOwnerOrManager": n.SpaceOwnerOrManager(ctx).GetOpaqueId(),
		},
	}

	if metadata != nil {
		info.MetaData["providerID"] = metadata["providerID"]
		if mtime, ok := metadata["mtime"]; ok {
			info.MetaData["mtime"] = mtime
		}
		if expiration, ok := metadata["expires"]; ok {
			info.MetaData["expires"] = expiration
		}
		if _, ok := metadata["sizedeferred"]; ok {
			info.SizeIsDeferred = true
		}
		if checksum, ok := metadata["checksum"]; ok {
			parts := strings.SplitN(checksum, " ", 2)
			if len(parts) != 2 {
				return nil, errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
			}
			switch parts[0] {
			case "sha1", "md5", "adler32":
				info.MetaData["checksum"] = checksum
			default:
				return nil, errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
			}
		}
		if ifMatch, ok := metadata["if-match"]; ok {
			info.MetaData["if-match"] = ifMatch
		}
	}

	log.Debug().Interface("info", info).Interface("node", n).Interface("metadata", metadata).Msg("Decomposedfs: resolved filename")

	_, err = node.CheckQuota(n.SpaceRoot, n.Exists, uint64(n.Blobsize), uint64(info.Size))
	if err != nil {
		return nil, err
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

// UseIn tells the tus upload middleware which extensions it supports.
func (fs *Decomposedfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}

// To implement the core tus.io protocol as specified in https://tus.io/protocols/resumable-upload.html#core-protocol
// - the storage needs to implement NewUpload and GetUpload
// - the upload needs to implement the tusd.Upload interface: WriteChunk, GetInfo, GetReader and FinishUpload

// NewUpload returns a new tus Upload instance
func (fs *Decomposedfs) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("Decomposedfs: NewUpload")

	if info.MetaData["filename"] == "" {
		return nil, errors.New("Decomposedfs: missing filename in metadata")
	}
	if info.MetaData["dir"] == "" {
		return nil, errors.New("Decomposedfs: missing dir in metadata")
	}

	n, err := fs.lu.NodeFromSpaceID(ctx, &provider.ResourceId{
		SpaceId:  info.Storage["SpaceRoot"],
		OpaqueId: info.Storage["SpaceRoot"],
	})
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error getting space root node")
	}

	n, err = fs.lookupNode(ctx, n, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]))
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error walking path")
	}

	log.Debug().Interface("info", info).Interface("node", n).Msg("Decomposedfs: resolved filename")

	// the parent owner will become the new owner
	p, perr := n.Parent()
	if perr != nil {
		return nil, errors.Wrap(perr, "Decomposedfs: error getting parent "+n.ParentID)
	}

	// check permissions
	var checkNode *node.Node
	var f string
	if n.Exists {
		// check permissions of file to be overwritten
		checkNode = n
		f, _ = storagespace.FormatReference(&provider.Reference{ResourceId: &provider.ResourceId{
			SpaceId:  n.SpaceID,
			OpaqueId: n.ID,
		}})
	} else {
		// check permissions of parent
		checkNode = p
		f, _ = storagespace.FormatReference(&provider.Reference{ResourceId: &provider.ResourceId{
			SpaceId:  p.SpaceID,
			OpaqueId: p.ID,
		}, Path: n.Name})
	}
	rp, err := fs.p.AssemblePermissions(ctx, checkNode)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.InitiateFileUpload:
		if rp.Stat {
			return nil, errtypes.PermissionDenied(f)
		}
		return nil, errtypes.NotFound(f)
	}

	// if we are trying to overwriting a folder with a file
	if n.Exists && n.IsDir() {
		return nil, errtypes.PreconditionFailed("resource is not a file")
	}

	// check lock
	if info.MetaData["lockid"] != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, info.MetaData["lockid"])
	}
	if err := n.CheckLock(ctx); err != nil {
		return nil, err
	}

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error resolving upload path")
	}
	usr := ctxpkg.ContextMustGetUser(ctx)

	spaceRoot := n.SpaceRoot.ID
	if info.Storage != nil && info.Storage["SpaceRoot"] != "" {
		spaceRoot = info.Storage["SpaceRoot"]
	}

	info.Storage = map[string]string{
		"Type":    "OCISStore",
		"BinPath": binPath,

		"NodeId":              n.ID,
		"NodeParentId":        n.ParentID,
		"NodeName":            n.Name,
		"SpaceRoot":           spaceRoot,
		"SpaceOwnerOrManager": info.Storage["SpaceOwnerOrManager"],

		"Idp":      usr.Id.Idp,
		"UserId":   usr.Id.OpaqueId,
		"UserType": utils.UserTypeToString(usr.Id.Type),
		"UserName": usr.Username,

		"LogLevel": log.GetLevel().String(),
	}
	// Create binary file in the upload folder with no content
	log.Debug().Interface("info", info).Msg("Decomposedfs: built storage info")
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	u := &fileUpload{
		info:     info,
		binPath:  binPath,
		infoPath: filepath.Join(fs.o.Root, "uploads", info.ID+".info"),
		fs:       fs,
		ctx:      ctx,
	}

	// writeInfo creates the file by itself if necessary
	err = u.writeInfo()
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (fs *Decomposedfs) getUploadPath(ctx context.Context, uploadID string) (string, error) {
	return filepath.Join(fs.o.Root, "uploads", uploadID), nil
}

func (fs *Decomposedfs) readInfo(id string) (tusd.FileInfo, error) {
	infoPath := filepath.Join(fs.o.Root, "uploads", id+".info")

	info := tusd.FileInfo{}
	data, err := os.ReadFile(infoPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = tusd.ErrNotFound
		}
		return tusd.FileInfo{}, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return tusd.FileInfo{}, err
	}

	stat, err := os.Stat(info.Storage["BinPath"])
	if err != nil {
		return tusd.FileInfo{}, err
	}
	info.Offset = stat.Size()

	return info, nil
}

// GetUpload returns the Upload for the given upload id
func (fs *Decomposedfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	l := appctx.GetLogger(ctx)
	sub := l.With().Int("pid", os.Getpid()).Logger()
	info, err := fs.readInfo(id)
	if err != nil {
		return nil, err
	}

	u := &userpb.User{
		Id: &userpb.UserId{
			Idp:      info.Storage["Idp"],
			OpaqueId: info.Storage["UserId"],
			Type:     utils.UserTypeMap(info.Storage["UserType"]),
		},
		Username: info.Storage["UserName"],
	}

	ctx = ctxpkg.ContextSetUser(ctx, u)
	ctx = appctx.WithLogger(ctx, &sub)

	return &fileUpload{
		info:     info,
		binPath:  info.Storage["BinPath"],
		infoPath: filepath.Join(fs.o.Root, "uploads", id+".info"),
		fs:       fs,
		ctx:      ctx,
	}, nil
}

// ListUploads returns a list of all incomplete uploads
func (fs *Decomposedfs) ListUploads() ([]tusd.FileInfo, error) {
	return fs.uploadInfos()
}

func (fs *Decomposedfs) uploadInfos() ([]tusd.FileInfo, error) {
	infos := []tusd.FileInfo{}
	infoFiles, err := filepath.Glob(filepath.Join(fs.o.Root, "uploads", "*.info"))
	if err != nil {
		return nil, err
	}

	idRegexp := regexp.MustCompile(".*/([^/]+).info")
	for _, info := range infoFiles {
		match := idRegexp.FindStringSubmatch(info)
		if match == nil || len(match) < 2 {
			continue
		}
		info, err := fs.readInfo(match[1])
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// PurgeExpiredUploads scans the fs for expired downloads and removes any leftovers
func (fs *Decomposedfs) PurgeExpiredUploads(purgedChan chan<- tusd.FileInfo) error {
	infos, err := fs.uploadInfos()
	if err != nil {
		return err
	}

	for _, info := range infos {
		expires, err := strconv.Atoi(info.MetaData["expires"])
		if err != nil {
			continue
		}
		if int64(expires) < time.Now().Unix() {
			purgedChan <- info
			err = os.Remove(info.Storage["BinPath"])
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

// lookupNode looks up nodes by path.
// This method can also handle lookups for paths which contain chunking information.
func (fs *Decomposedfs) lookupNode(ctx context.Context, spaceRoot *node.Node, path string) (*node.Node, error) {
	p := path
	isChunked := chunking.IsChunked(path)
	if isChunked {
		chunkInfo, err := chunking.GetChunkBLOBInfo(path)
		if err != nil {
			return nil, err
		}
		p = chunkInfo.Path
	}

	n, err := fs.lu.WalkPath(ctx, spaceRoot, p, true, func(ctx context.Context, n *node.Node) error { return nil })
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error walking path")
	}

	if isChunked {
		n.Name = filepath.Base(path)
	}
	return n, nil
}

type fileUpload struct {
	// info stores the current information about the upload
	info tusd.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
	// only fs knows how to handle metadata and versions
	fs *Decomposedfs
	// a context with a user
	// TODO add logger as well?
	ctx context.Context
}

// GetInfo returns the FileInfo
func (upload *fileUpload) GetInfo(ctx context.Context) (tusd.FileInfo, error) {
	return upload.info, nil
}

// WriteChunk writes the stream from the reader to the given offset of the upload
func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// calculate cheksum here? needed for the TUS checksum extension. https://tus.io/protocols/resumable-upload.html#checksum
	// TODO but how do we get the `Upload-Checksum`? WriteChunk() only has a context, offset and the reader ...
	// It is sent with the PATCH request, well or in the POST when the creation-with-upload extension is used
	// but the tus handler uses a context.Background() so we cannot really check the header and put it in the context ...
	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for the ocis driver it's not important whether the stream has ended
	// on purpose or accidentally.
	if err != nil {
		if err != io.ErrUnexpectedEOF {
			return n, err
		}
	}

	upload.info.Offset += n
	err = upload.writeInfo() // TODO info is written here ... we need to truncate in DiscardChunk

	return n, err
}

// GetReader returns an io.Reader for the upload
func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return os.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the internal destination
//
// # upload steps
// check if match header to fail early
// copy blob
// lock metadata node
// check if match header again as safeguard
// read metadata
// create version node with current metadata
// update node metadata with new blobid etc
// remember size diff
// unlock metadata
// propagate size diff and new etag
// - propagation can happen outside the metadata lock because diff calculation happens inside the lock and the order in which diffs are applied to the parent is irrelvevant
// - propagation needs to propagate the diff
func (upload *fileUpload) FinishUpload(ctx context.Context) (err error) {

	// ensure cleanup
	defer upload.discardChunk()

	fi, err := os.Stat(upload.binPath)
	if err != nil {
		appctx.GetLogger(upload.ctx).Err(err).Msg("Decomposedfs: could not stat uploaded file")
		return
	}

	spaceID := upload.info.Storage["SpaceRoot"]
	n := node.New(
		spaceID,
		upload.info.Storage["NodeId"],
		upload.info.Storage["NodeParentId"],
		upload.info.Storage["NodeName"],
		fi.Size(),
		"",
		nil,
		upload.fs.lu,
	)
	n.SpaceRoot = node.New(spaceID, spaceID, "", "", 0, "", nil, upload.fs.lu)

	// check lock
	if upload.info.MetaData["lockid"] != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, upload.info.MetaData["lockid"])
	}
	if err := n.CheckLock(ctx); err != nil {
		return err
	}

	overwrite := n.ID != ""
	var oldSize int64
	if overwrite {
		// read size from existing node
		old, _ := node.ReadNode(ctx, upload.fs.lu, spaceID, n.ID, false)
		oldSize = old.Blobsize
	} else {
		// create new fileid
		n.ID = uuid.New().String()
		upload.info.Storage["NodeId"] = n.ID
	}

	if _, err = node.CheckQuota(n.SpaceRoot, overwrite, uint64(oldSize), uint64(fi.Size())); err != nil {
		return err
	}

	targetPath := n.InternalPath()
	sublog := appctx.GetLogger(upload.ctx).
		With().
		Interface("info", upload.info).
		Str("spaceid", spaceID).
		Str("nodeid", n.ID).
		Str("binPath", upload.binPath).
		Str("targetPath", targetPath).
		Logger()

	// calculate the checksum of the written bytes
	// they will all be written to the metadata later, so we cannot omit any of them
	// TODO only calculate the checksum in sync that was requested to match, the rest could be async ... but the tests currently expect all to be present
	// TODO the hashes all implement BinaryMarshaler so we could try to persist the state for resumable upload. we would neet do keep track of the copied bytes ...
	sha1h := sha1.New()
	md5h := md5.New()
	adler32h := adler32.New()
	{
		f, err := os.Open(upload.binPath)
		if err != nil {
			sublog.Err(err).Msg("Decomposedfs: could not open file for checksumming")
			// we can continue if no oc checksum header is set
		}
		defer f.Close()

		r1 := io.TeeReader(f, sha1h)
		r2 := io.TeeReader(r1, md5h)

		if _, err := io.Copy(adler32h, r2); err != nil {
			sublog.Err(err).Msg("Decomposedfs: could not copy bytes for checksumming")
		}
	}
	// compare if they match the sent checksum
	// TODO the tus checksum extension would do this on every chunk, but I currently don't see an easy way to pass in the requested checksum. for now we do it in FinishUpload which is also called for chunked uploads
	if upload.info.MetaData["checksum"] != "" {
		parts := strings.SplitN(upload.info.MetaData["checksum"], " ", 2)
		if len(parts) != 2 {
			return errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
		}
		switch parts[0] {
		case "sha1":
			err = upload.checkHash(parts[1], sha1h)
		case "md5":
			err = upload.checkHash(parts[1], md5h)
		case "adler32":
			err = upload.checkHash(parts[1], adler32h)
		default:
			err = errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
		}
		if err != nil {
			return err
		}
	}
	n.BlobID = upload.info.ID // This can be changed to a content hash in the future when reference counting for the blobs was added

	// defer writing the checksums until the node is in place

	// upload steps
	// check if match header to fail early

	if fi, err = os.Stat(targetPath); err == nil {
		// When the if-match header was set we need to check if the
		// etag still matches before finishing the upload.
		if ifMatch, ok := upload.info.MetaData["if-match"]; ok {
			var targetEtag string
			targetEtag, err = node.CalculateEtag(n.ID, fi.ModTime())
			if err != nil {
				return errtypes.InternalError(err.Error())
			}
			if ifMatch != targetEtag {
				return errtypes.Aborted("etag mismatch")
			}
		}
	} else {
		// create dir to node
		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			sublog.Err(err).Msg("could not create node dir")
			return errtypes.InternalError("could not create node dir")
		}
	}

	// copy blob

	file, err := os.Open(upload.binPath)
	if err != nil {
		return err
	}
	defer file.Close()
	err = upload.fs.tp.WriteBlob(n, file)
	if err != nil {
		return errors.Wrap(err, "failed to upload file to blobstore")
	}

	// prepare discarding the blob if something changed while writing it
	discardBlob := func() {
		if err := upload.fs.tp.DeleteBlob(n); err != nil {
			sublog.Err(err).Str("blobid", n.BlobID).Msg("Decomposedfs: failed to discard blob in blobstore")
		}
	}

	// lock metadata node
	lock, err := filelocks.AcquireWriteLock(targetPath)
	if err != nil {
		discardBlob()
		return errtypes.InternalError(err.Error())
	}
	releaseLock := func() {
		// ReleaseLock returns nil if already unlocked
		if err := filelocks.ReleaseLock(lock); err != nil {
			sublog.Err(err).Msg("Decomposedfs:could not unlock node")
		}
	}
	defer releaseLock()

	// check if match header again as safeguard
	var oldMtime time.Time
	versionsPath := ""
	if fi, err = os.Stat(targetPath); err == nil {
		// When the if-match header was set we need to check if the
		// etag still matches before finishing the upload.
		if ifMatch, ok := upload.info.MetaData["if-match"]; ok {
			var targetEtag string
			targetEtag, err = node.CalculateEtag(n.ID, fi.ModTime())
			if err != nil {
				discardBlob()
				return errtypes.InternalError(err.Error())
			}
			if ifMatch != targetEtag {
				discardBlob()
				return errtypes.Aborted("etag mismatch")
			}
		}

		// versions are stored alongside the actual file, so a rename can be efficient and does not cross storage / partition boundaries
		versionsPath = upload.fs.lu.InternalPath(spaceID, n.ID+node.RevisionIDDelimiter+fi.ModTime().UTC().Format(time.RFC3339Nano))

		// remember mtime of existing file so we can apply it to the version
		oldMtime = fi.ModTime()
	}

	// read metadata

	// attributes that will change
	attrs := map[string]string{
		xattrs.BlobIDAttr:   n.BlobID,
		xattrs.BlobsizeAttr: strconv.FormatInt(n.Blobsize, 10),

		// update checksums
		xattrs.ChecksumPrefix + "sha1":    string(sha1h.Sum(nil)),
		xattrs.ChecksumPrefix + "md5":     string(md5h.Sum(nil)),
		xattrs.ChecksumPrefix + "adler32": string(adler32h.Sum(nil)),
	}

	// create version node with current metadata

	var newMtime time.Time
	// if file already exists
	if versionsPath != "" {
		// touch version node
		file, err := os.Create(versionsPath)
		if err != nil {
			discardBlob()
			sublog.Err(err).Str("version", versionsPath).Msg("could not create version node")
			return errtypes.InternalError("could not create version node")
		}
		fi, err := file.Stat()
		if err != nil {
			file.Close()
			discardBlob()
			sublog.Err(err).Str("version", versionsPath).Msg("could not stat version node")
			return errtypes.InternalError("could not stat version node")
		}
		newMtime = fi.ModTime()
		file.Close()

		// copy blob metadata to version node
		err = xattrs.CopyMetadataWithSourceLock(targetPath, versionsPath, func(attributeName string) bool {
			return strings.HasPrefix(attributeName, xattrs.ChecksumPrefix) ||
				attributeName == xattrs.BlobIDAttr ||
				attributeName == xattrs.BlobsizeAttr
		}, lock)
		if err != nil {
			discardBlob()
			sublog.Err(err).Str("version", versionsPath).Msg("failed to copy xattrs to version node")
			return errtypes.InternalError("failed to copy blob xattrs to version node")
		}

		// keep mtime from previous version
		if err := os.Chtimes(versionsPath, oldMtime, oldMtime); err != nil {
			discardBlob()
			sublog.Err(err).Str("version", versionsPath).Msg("failed to change mtime of version node")
			return errtypes.InternalError("failed to change mtime of version node")
		}

		// we MUST bypass any cache here as we have to calculate the size diff atomically
		oldSize, err = node.ReadBlobSizeAttr(targetPath)
		if err != nil {
			discardBlob()
			sublog.Err(err).Str("version", versionsPath).Msg("failed to read old blobsize")
			return errtypes.InternalError("failed to read old blobsize")
		}
	} else {
		// touch metadata node
		file, err := os.Create(targetPath)
		if err != nil {
			discardBlob()
			sublog.Err(err).Msg("could not create node")
			return errtypes.InternalError("could not create node")
		}
		file.Close()

		// basic node metadata
		attrs[xattrs.ParentidAttr] = n.ParentID
		attrs[xattrs.NameAttr] = n.Name
		oldSize = 0
	}

	// update node metadata with new blobid etc
	err = n.SetXattrsWithLock(attrs, lock)
	if err != nil {
		discardBlob()
		return errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	// update mtime
	switch {
	case upload.info.MetaData["mtime"] != "":
		if err := n.SetMtimeString(upload.info.MetaData["mtime"]); err != nil {
			sublog.Err(err).Interface("info", upload.info).Msg("Decomposedfs: could not apply mtime from metadata")
			return err
		}
	case newMtime != time.Time{}:
		// we are creating a version
		if err := n.SetMtime(newMtime); err != nil {
			sublog.Err(err).Interface("info", upload.info).Msg("Decomposedfs: could not change mtime of node")
			return err
		}
	}

	// remember size diff
	sizeDiff := oldSize - n.Blobsize

	// unlock metadata
	err = filelocks.ReleaseLock(lock)
	if err != nil {
		return errtypes.InternalError(err.Error())
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentInternalPath(), n.Name)
	relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
	var link string
	link, err = os.Readlink(childNameLink)
	if err == nil && link != relativeNodePath {
		sublog.Err(err).
			Interface("node", n).
			Str("childNameLink", childNameLink).
			Str("link", link).
			Msg("Decomposedfs: child name link has wrong target id, repairing")

		if err = os.Remove(childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not remove symlink child entry")
		}
	}
	if errors.Is(err, iofs.ErrNotExist) || link != relativeNodePath {
		if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not symlink child entry")
		}
	}

	// fill metadata with current mtime
	if fi, err = os.Stat(targetPath); err == nil {
		upload.info.MetaData["mtime"] = fmt.Sprintf("%d.%d", fi.ModTime().Unix(), fi.ModTime().Nanosecond())
		upload.info.MetaData["etag"], _ = node.CalculateEtag(n.ID, fi.ModTime())
	}

	n.Exists = true

	// propagate size diff and new etag
	//   propagation can happen outside the metadata lock because diff calculation happens inside the lock and the order in which diffs are applied to the parent is irrelvevant
	//   propagation needs to propagate the diff

	// return upload.fs.tp.Propagate(upload.ctx, n, sizeDiff)
	sublog.Debug().Int64("sizediff", sizeDiff).Msg("Decomposedfs: propagating size diff")
	return upload.fs.tp.Propagate(upload.ctx, n)
}

func (upload *fileUpload) checkHash(expected string, h hash.Hash) error {
	if expected != hex.EncodeToString(h.Sum(nil)) {
		upload.discardChunk()
		return errtypes.ChecksumMismatch(fmt.Sprintf("invalid checksum: expected %s got %x", upload.info.MetaData["checksum"], h.Sum(nil)))
	}
	return nil
}

func (upload *fileUpload) discardChunk() {
	if err := os.Remove(upload.binPath); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			appctx.GetLogger(upload.ctx).Err(err).Interface("info", upload.info).Str("binPath", upload.binPath).Interface("info", upload.info).Msg("Decomposedfs: could not discard chunk")
			return
		}
	}
	if err := os.Remove(upload.infoPath); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			appctx.GetLogger(upload.ctx).Err(err).Interface("info", upload.info).Str("infoPath", upload.infoPath).Interface("info", upload.info).Msg("Decomposedfs: could not discard chunk info")
			return
		}
	}
}

// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// - the storage needs to implement AsTerminatableUpload
// - the upload needs to implement Terminate

// AsTerminatableUpload returns a TerminatableUpload
func (fs *Decomposedfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
	return upload.(*fileUpload)
}

// Terminate terminates the upload
func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.infoPath); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(upload.binPath); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// To implement the creation-defer-length extension as specified in https://tus.io/protocols/resumable-upload.html#creation
// - the storage needs to implement AsLengthDeclarableUpload
// - the upload needs to implement DeclareLength

// AsLengthDeclarableUpload returns a LengthDeclarableUpload
func (fs *Decomposedfs) AsLengthDeclarableUpload(upload tusd.Upload) tusd.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

// DeclareLength updates the upload length information
func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// To implement the concatenation extension as specified in https://tus.io/protocols/resumable-upload.html#concatenation
// - the storage needs to implement AsConcatableUpload
// - the upload needs to implement ConcatUploads

// AsConcatableUpload returns a ConcatableUpload
func (fs *Decomposedfs) AsConcatableUpload(upload tusd.Upload) tusd.ConcatableUpload {
	return upload.(*fileUpload)
}

// ConcatUploads concatenates multiple uploads
func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []tusd.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(fileUpload.binPath)
		if err != nil {
			return err
		}
		defer src.Close()

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}
