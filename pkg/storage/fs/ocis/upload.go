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
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

func (fs *ocisfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) (err error) {
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
			return errors.Wrap(err, "ocisfs: error retrieving upload")
		}
	}

	uploadInfo := upload.(*fileUpload)

	p := uploadInfo.info.Storage["NodeName"]
	ok, err := chunking.IsChunked(p)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error checking path")
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
		uploadInfo.info.Storage["NodeName"] = p
		fd, err := os.Open(assembledFile)
		if err != nil {
			return errors.Wrap(err, "ocisfs: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	if _, err := uploadInfo.WriteChunk(ctx, 0, r); err != nil {
		return errors.Wrap(err, "ocisfs: error writing to binary file")
	}

	return uploadInfo.FinishUpload(ctx)
}

// InitiateUpload returns upload ids corresponding to different protocols it supports
// TODO read optional content for small files in this request
func (fs *ocisfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {

	log := appctx.GetLogger(ctx)

	var relative string // the internal path of the file node

	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return nil, err
	}

	// permissions are checked in NewUpload below

	relative, err = fs.lu.Path(ctx, n)
	if err != nil {
		return nil, err
	}

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(relative),
			"dir":      filepath.Dir(relative),
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

	log.Debug().Interface("info", info).Interface("node", n).Interface("metadata", metadata).Msg("ocisfs: resolved filename")

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
func (fs *ocisfs) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(fs)
	composer.UseTerminater(fs)
	composer.UseConcater(fs)
	composer.UseLengthDeferrer(fs)
}

// To implement the core tus.io protocol as specified in https://tus.io/protocols/resumable-upload.html#core-protocol
// - the storage needs to implement NewUpload and GetUpload
// - the upload needs to implement the tusd.Upload interface: WriteChunk, GetInfo, GetReader and FinishUpload

func (fs *ocisfs) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("ocisfs: NewUpload")

	fn := info.MetaData["filename"]
	if fn == "" {
		return nil, errors.New("ocisfs: missing filename in metadata")
	}
	info.MetaData["filename"] = filepath.Clean(info.MetaData["filename"])

	dir := info.MetaData["dir"]
	if dir == "" {
		return nil, errors.New("ocisfs: missing dir in metadata")
	}
	info.MetaData["dir"] = filepath.Clean(info.MetaData["dir"])

	n, err := fs.lu.NodeFromPath(ctx, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]))
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error wrapping filename")
	}

	log.Debug().Interface("info", info).Interface("node", n).Msg("ocisfs: resolved filename")

	// check permissions
	var ok bool
	if n.Exists {
		// check permissions of file to be overwritten
		ok, err = fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
			return rp.InitiateFileUpload
		})
	} else {
		// check permissions of parent
		p, perr := n.Parent()
		if perr != nil {
			return nil, errors.Wrap(perr, "ocisfs: error getting parent "+n.ParentID)
		}

		ok, err = fs.p.HasPermission(ctx, p, func(rp *provider.ResourcePermissions) bool {
			return rp.InitiateFileUpload
		})
	}
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error resolving upload path")
	}
	usr := user.ContextMustGetUser(ctx)
	info.Storage = map[string]string{
		"Type":    "OCISStore",
		"BinPath": binPath,

		"NodeId":       n.ID,
		"NodeParentId": n.ParentID,
		"NodeName":     n.Name,

		"Idp":      usr.Id.Idp,
		"UserId":   usr.Id.OpaqueId,
		"UserName": usr.Username,

		"LogLevel": log.GetLevel().String(),
	}
	// Create binary file in the upload folder with no content
	log.Debug().Interface("info", info).Msg("ocisfs: built storage info")
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

	if !info.SizeIsDeferred && info.Size == 0 {
		log.Debug().Interface("info", info).Msg("ocisfs: finishing upload for empty file")
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

func (fs *ocisfs) getUploadPath(ctx context.Context, uploadID string) (string, error) {
	return filepath.Join(fs.o.Root, "uploads", uploadID), nil
}

// GetUpload returns the Upload for the given upload id
func (fs *ocisfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	infoPath := filepath.Join(fs.o.Root, "uploads", id+".info")

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
	fs *ocisfs
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
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *fileUpload) FinishUpload(ctx context.Context) (err error) {
	log := appctx.GetLogger(upload.ctx)

	n := &Node{
		lu:       upload.fs.lu,
		ID:       upload.info.Storage["NodeId"],
		ParentID: upload.info.Storage["NodeParentId"],
		Name:     upload.info.Storage["NodeName"],
	}

	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	targetPath := upload.fs.lu.toInternalPath(n.ID)

	// if target exists create new version
	var fi os.FileInfo
	if fi, err = os.Stat(targetPath); err == nil {
		// versions are stored alongside the actual file, so a rename can be efficient and does not cross storage / partition boundaries
		versionsPath := upload.fs.lu.toInternalPath(n.ID + ".REV." + fi.ModTime().UTC().Format(time.RFC3339Nano))

		if err = os.Rename(targetPath, versionsPath); err != nil {
			log.Err(err).Interface("info", upload.info).
				Str("binPath", upload.binPath).
				Str("targetPath", targetPath).
				Msg("ocisfs: could not create version")
			return
		}
	}

	// now rename the upload to the target path
	// TODO put uploads on the same underlying storage as the destination dir?
	// TODO trigger a workflow as the final rename might eg involve antivirus scanning
	if err = os.Rename(upload.binPath, targetPath); err != nil {
		log := appctx.GetLogger(upload.ctx)
		log.Err(err).Interface("info", upload.info).
			Str("binPath", upload.binPath).
			Str("targetPath", targetPath).
			Msg("ocisfs: could not rename")
		return
	}
	// who will become the owner?
	u, ok := user.ContextGetUser(upload.ctx)
	switch {
	case ok:
		err = n.writeMetadata(u.Id)
	case upload.fs.o.EnableHome:
		log := appctx.GetLogger(upload.ctx)
		log.Error().Msg("home support enabled but no user in context")
		err = errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from upload ctx")
	case upload.fs.o.Owner != "":
		err = n.writeMetadata(&userpb.UserId{
			OpaqueId: upload.fs.o.Owner,
		})
	default:
		// fallback to parent owner?
		err = n.writeMetadata(nil)
	}
	if err != nil {
		return
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(upload.fs.lu.toInternalPath(n.ParentID), n.Name)
	var link string
	link, err = os.Readlink(childNameLink)
	if err == nil && link != "../"+n.ID {
		log.Err(err).
			Interface("info", upload.info).
			Interface("node", n).
			Str("targetPath", targetPath).
			Str("childNameLink", childNameLink).
			Str("link", link).
			Msg("ocisfs: child name link has wrong target id, repairing")

		if err = os.Remove(childNameLink); err != nil {
			return errors.Wrap(err, "ocisfs: could not remove symlink child entry")
		}
	}
	if os.IsNotExist(err) || link != "../"+n.ID {
		if err = os.Symlink("../"+n.ID, childNameLink); err != nil {
			return errors.Wrap(err, "ocisfs: could not symlink child entry")
		}
	}

	// only delete the upload if it was successfully written to the storage
	if err = os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			log.Err(err).Interface("info", upload.info).Msg("ocisfs: could not delete upload info")
			return
		}
	}
	// use set arbitrary metadata?
	/*if upload.info.MetaData["mtime"] != "" {
		err := upload.fs.SetMtime(ctx, np, upload.info.MetaData["mtime"])
		if err != nil {
			log.Err(err).Interface("info", upload.info).Msg("ocisfs: could not set mtime metadata")
			return err
		}
	}*/

	n.Exists = true

	return upload.fs.tp.Propagate(upload.ctx, n)
}

// To implement the termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// - the storage needs to implement AsTerminatableUpload
// - the upload needs to implement Terminate

// AsTerminatableUpload returns a TerminatableUpload
func (fs *ocisfs) AsTerminatableUpload(upload tusd.Upload) tusd.TerminatableUpload {
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
func (fs *ocisfs) AsLengthDeclarableUpload(upload tusd.Upload) tusd.LengthDeclarableUpload {
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
func (fs *ocisfs) AsConcatableUpload(upload tusd.Upload) tusd.ConcatableUpload {
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
