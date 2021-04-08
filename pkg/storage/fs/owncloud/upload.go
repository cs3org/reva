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

package owncloud

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

func (fs *ocfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	upload, err := fs.GetUpload(ctx, ref.GetPath())
	if err != nil {
		// Upload corresponding to this ID was not found.
		// Assume that this corresponds to the resource path to which the file has to be uploaded.

		// Set the length to 0 and set SizeIsDeferred to true
		metadata := map[string]string{"sizedeferred": "true"}
		uploadIDs, err := fs.InitiateUpload(ctx, ref, 0, metadata)
		if err != nil {
			return err
		}
		if upload, err = fs.GetUpload(ctx, uploadIDs["simple"]); err != nil {
			return errors.Wrap(err, "ocfs: error retrieving upload")
		}
	}

	uploadInfo := upload.(*fileUpload)

	p := uploadInfo.info.Storage["InternalDestination"]
	ok, err := chunking.IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "ocfs: error checking path")
	}
	if ok {
		var assembledFile string
		p, assembledFile, err = fs.chunkHandler.WriteChunk(p, r)
		if err != nil {
			return err
		}
		if p == "" {
			if err = uploadInfo.Terminate(ctx); err != nil {
				return errors.Wrap(err, "ocfs: error removing auxiliary files")
			}
			return errtypes.PartialContent(ref.String())
		}
		uploadInfo.info.Storage["InternalDestination"] = p
		fd, err := os.Open(assembledFile)
		if err != nil {
			return errors.Wrap(err, "ocfs: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	if _, err := uploadInfo.WriteChunk(ctx, 0, r); err != nil {
		return errors.Wrap(err, "ocfs: error writing to binary file")
	}

	return uploadInfo.FinishUpload(ctx)
}

// InitiateUpload returns upload ids corresponding to different protocols it supports
// TODO read optional content for small files in this request
func (fs *ocfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}

	// permissions are checked in NewUpload below

	p := fs.toStoragePath(ctx, ip)

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(p),
			"dir":      filepath.Dir(p),
		},
		Size: uploadLength,
	}

	if metadata != nil {
		if metadata["mtime"] != "" {
			info.MetaData["mtime"] = metadata["mtime"]
		}
		if _, ok := metadata["sizedeferred"]; ok {
			info.SizeIsDeferred = true
		}
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
func (fs *ocfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}

// To implement the core tus.io protocol as specified in https://tus.io/protocols/resumable-upload.html#core-protocol
// - the storage needs to implement NewUpload and GetUpload
// - the upload needs to implement the tusd.Upload interface: WriteChunk, GetInfo, GetReader and FinishUpload

func (fs *ocfs) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("ocfs: NewUpload")

	if info.MetaData["filename"] == "" {
		return nil, errors.New("ocfs: missing filename in metadata")
	}
	info.MetaData["filename"] = filepath.Clean(info.MetaData["filename"])

	dir := info.MetaData["dir"]
	if dir == "" {
		return nil, errors.New("ocfs: missing dir in metadata")
	}
	info.MetaData["dir"] = filepath.Clean(info.MetaData["dir"])

	ip := fs.toInternalPath(ctx, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]))

	// check permissions
	var perm *provider.ResourcePermissions
	var perr error
	// if destination exists
	if _, err := os.Stat(ip); err == nil {
		// check permissions of file to be overwritten
		perm, perr = fs.readPermissions(ctx, ip)
	} else {
		// check permissions of parent folder
		perm, perr = fs.readPermissions(ctx, filepath.Dir(ip))
	}
	if perr == nil {
		if !perm.InitiateFileUpload {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	log.Debug().Interface("info", info).Msg("ocfs: resolved filename")

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving upload path")
	}
	usr := user.ContextMustGetUser(ctx)
	info.Storage = map[string]string{
		"Type":                "OwnCloudStore",
		"BinPath":             binPath,
		"InternalDestination": ip,

		"Idp":      usr.Id.Idp,
		"UserId":   usr.Id.OpaqueId,
		"UserName": usr.Username,

		"LogLevel": log.GetLevel().String(),
	}
	// Create binary file in the upload folder with no content
	log.Debug().Interface("info", info).Msg("ocfs: built storage info")
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	u := &fileUpload{
		info:     info,
		binPath:  binPath,
		infoPath: filepath.Join(fs.c.UploadInfoDir, info.ID+".info"),
		fs:       fs,
		ctx:      ctx,
	}

	if !info.SizeIsDeferred && info.Size == 0 {
		log.Debug().Interface("info", info).Msg("ocfs: finishing upload for empty file")
		// no need to create info file and finish directly
		err := u.FinishUpload(ctx)
		if err != nil {
			return nil, err
		}
		return u, nil
	}

	// writeInfo creates the file by itself if necessary
	err = u.writeInfo()
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (fs *ocfs) getUploadPath(ctx context.Context, uploadID string) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return "", err
	}
	layout := templates.WithUser(u, fs.c.UserLayout)
	return filepath.Join(fs.c.DataDirectory, layout, "uploads", uploadID), nil
}

// GetUpload returns the Upload for the given upload id
func (fs *ocfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	infoPath := filepath.Join(fs.c.UploadInfoDir, id+".info")

	info := tusd.FileInfo{}
	data, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	stat, err := os.Stat(info.Storage["BinPath"])
	if err != nil {
		return nil, err
	}

	info.Offset = stat.Size()

	u := &userpb.User{
		Id: &userpb.UserId{
			Idp:      info.Storage["Idp"],
			OpaqueId: info.Storage["UserId"],
		},
		Username: info.Storage["UserName"],
	}

	ctx = user.ContextSetUser(ctx, u)
	// TODO configure the logger the same way ... store and add traceid in file info

	var opts []logger.Option
	opts = append(opts, logger.WithLevel(info.Storage["LogLevel"]))
	opts = append(opts, logger.WithWriter(os.Stderr, logger.ConsoleMode))
	l := logger.New(opts...)

	sub := l.With().Int("pid", os.Getpid()).Logger()

	ctx = appctx.WithLogger(ctx, &sub)

	return &fileUpload{
		info:     info,
		binPath:  info.Storage["BinPath"],
		infoPath: infoPath,
		fs:       fs,
		ctx:      ctx,
	}, nil
}

type fileUpload struct {
	// info stores the current information about the upload
	info tusd.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
	// only fs knows how to handle metadata and versions
	fs *ocfs
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

	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for OwnCloudStore it's not important whether the stream has ended
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
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	log := appctx.GetLogger(upload.ctx)

	sha1Sum := make([]byte, 0, 32)
	md5Sum := make([]byte, 0, 32)
	adler32Sum := make([]byte, 0, 32)
	{
		sha1h := sha1.New()
		md5h := md5.New()
		adler32h := adler32.New()
		f, err := os.Open(upload.binPath)
		if err != nil {
			log.Err(err).Msg("Decomposedfs: could not open file for checksumming")
			// we can continue if no oc checksum header is set
		}
		defer f.Close()

		r1 := io.TeeReader(f, sha1h)
		r2 := io.TeeReader(r1, md5h)

		if _, err := io.Copy(adler32h, r2); err != nil {
			log.Err(err).Msg("Decomposedfs: could not copy bytes for checksumming")
		}

		sha1Sum = sha1h.Sum(sha1Sum)
		md5Sum = md5h.Sum(md5Sum)
		adler32Sum = adler32h.Sum(adler32Sum)
	}

	if upload.info.MetaData["checksum"] != "" {
		parts := strings.SplitN(upload.info.MetaData["checksum"], " ", 2)
		if len(parts) != 2 {
			return errtypes.BadRequest("invalid checksum format. must be '[algorithm] [checksum]'")
		}
		var err error
		switch parts[0] {
		case "sha1":
			err = upload.checkHash(parts[1], sha1Sum)
		case "md5":
			err = upload.checkHash(parts[1], md5Sum)
		case "adler32":
			err = upload.checkHash(parts[1], adler32Sum)
		default:
			err = errtypes.BadRequest("unsupported checksum algorithm: " + parts[0])
		}
		if err != nil {
			return err
		}
	}

	ip := upload.info.Storage["InternalDestination"]

	// if destination exists
	// TODO check etag with If-Match header
	if _, err := os.Stat(ip); err == nil {
		// copy attributes of existing file to tmp file
		if err := upload.fs.copyMD(ip, upload.binPath); err != nil {
			return errors.Wrap(err, "ocfs: error copying metadata from "+ip+" to "+upload.binPath)
		}
		// create revision
		if err := upload.fs.archiveRevision(upload.ctx, upload.fs.getVersionsPath(upload.ctx, ip), ip); err != nil {
			return err
		}
	}

	err := os.Rename(upload.binPath, ip)
	if err != nil {
		log.Err(err).Interface("info", upload.info).
			Str("binPath", upload.binPath).
			Str("ipath", ip).
			Msg("ocfs: could not rename")
		return err
	}

	// only delete the upload if it was successfully written to the storage
	if err := os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			log.Err(err).Interface("info", upload.info).Msg("ocfs: could not delete upload info")
			return err
		}
	}

	if upload.info.MetaData["mtime"] != "" {
		err := upload.fs.setMtime(ctx, ip, upload.info.MetaData["mtime"])
		if err != nil {
			log.Err(err).Interface("info", upload.info).Msg("ocfs: could not set mtime metadata")
			return err
		}
	}

	// now try write all checksums
	tryWritingChecksum(log, ip, "sha1", sha1Sum)
	tryWritingChecksum(log, ip, "md5", md5Sum)
	tryWritingChecksum(log, ip, "adler32", adler32Sum)

	return upload.fs.propagate(upload.ctx, ip)
}

// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// - the storage needs to implement AsTerminatableUpload
// - the upload needs to implement Terminate

// AsTerminatableUpload returns a TerminatableUpload
func (fs *ocfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
	return upload.(*fileUpload)
}

// Terminate terminates the upload
func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(upload.binPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// To implement the creation-defer-length extension as specified in https://tus.io/protocols/resumable-upload.html#creation
// - the storage needs to implement AsLengthDeclarableUpload
// - the upload needs to implement DeclareLength

// AsLengthDeclarableUpload returns a LengthDeclarableUpload
func (fs *ocfs) AsLengthDeclarableUpload(upload tusd.Upload) tusd.LengthDeclarableUpload {
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
func (fs *ocfs) AsConcatableUpload(upload tusd.Upload) tusd.ConcatableUpload {
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

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) checkHash(expected string, h []byte) error {
	if expected != hex.EncodeToString(h) {
		upload.discardChunk()
		return errtypes.ChecksumMismatch(fmt.Sprintf("invalid checksum: expected %s got %x", upload.info.MetaData["checksum"], h))
	}
	return nil
}

func (upload *fileUpload) discardChunk() {
	if err := os.Remove(upload.binPath); err != nil {
		if !os.IsNotExist(err) {
			appctx.GetLogger(upload.ctx).Err(err).Interface("info", upload.info).Str("binPath", upload.binPath).Interface("info", upload.info).Msg("Decomposedfs: could not discard chunk")
			return
		}
	}
	if err := os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			appctx.GetLogger(upload.ctx).Err(err).Interface("info", upload.info).Str("infoPath", upload.infoPath).Interface("info", upload.info).Msg("Decomposedfs: could not discard chunk info")
			return
		}
	}
}

func tryWritingChecksum(log *zerolog.Logger, path, algo string, h []byte) {
	if err := xattr.Set(path, checksumPrefix+algo, h); err != nil {
		log.Err(err).
			Str("csType", algo).
			Bytes("hash", h).
			Msg("ocfs: could not write checksum")
	}
}
