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
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// TODO deprecated ... use tus

func (fs *ocisfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {

	node, err := fs.pw.NodeFromResource(ctx, ref)
	if err != nil {
		return err
	}

	if node.ID == "" {
		node.ID = uuid.New().String()
	}

	nodePath := filepath.Join(fs.conf.Root, "nodes", node.ID)

	tmp, err := ioutil.TempFile(nodePath, "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocisfs: error creating tmp fn at "+nodePath)
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error writing to tmp file "+tmp.Name())
	}

	// TODO move old content to version
	//_ = os.RemoveAll(path.Join(nodePath, "content"))

	err = os.Rename(tmp.Name(), nodePath)
	if err != nil {
		return err
	}
	return fs.tp.Propagate(ctx, node)

}

// InitiateUpload returns an upload id that can be used for uploads with tus
// TODO read optional content for small files in this request
func (fs *ocisfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (uploadID string, err error) {

	log := appctx.GetLogger(ctx)

	var relative string // the internal path of the file node

	node, err := fs.pw.NodeFromResource(ctx, ref)
	if err != nil {
		return "", err
	}

	relative, err = fs.pw.Path(ctx, node)
	if err != nil {
		return "", err
	}

	info := tusd.FileInfo{
		MetaData: tusd.MetaData{
			"filename": filepath.Base(relative),
			"dir":      filepath.Dir(relative),
		},
		Size: uploadLength,
	}

	if metadata != nil && metadata["mtime"] != "" {
		info.MetaData["mtime"] = metadata["mtime"]
	}

	log.Debug().Interface("info", info).Interface("node", node).Interface("metadata", metadata).Msg("ocisfs: resolved filename")

	upload, err := fs.NewUpload(ctx, info)
	if err != nil {
		return "", err
	}

	info, _ = upload.GetInfo(ctx)

	return info.ID, nil
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

	node, err := fs.pw.NodeFromPath(ctx, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]))
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error wrapping filename")
	}

	log.Debug().Interface("info", info).Interface("node", node).Msg("ocisfs: resolved filename")

	info.ID = uuid.New().String()

	binPath, err := fs.getUploadPath(ctx, info.ID)
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error resolving upload path")
	}
	usr := user.ContextMustGetUser(ctx)
	info.Storage = map[string]string{
		"Type":    "OCISStore",
		"BinPath": binPath,

		"NodeId":       node.ID,
		"NodeParentId": node.ParentID,
		"NodeName":     node.Name,

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
		infoPath: filepath.Join(fs.conf.Root, "uploads", info.ID+".info"),
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
	return filepath.Join(fs.conf.Root, "uploads", uploadID), nil
}

// GetUpload returns the Upload for the given upload id
func (fs *ocisfs) GetUpload(ctx context.Context, id string) (tusd.Upload, error) {
	infoPath := filepath.Join(fs.conf.Root, "uploads", id+".info")

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
func (upload *fileUpload) FinishUpload(ctx context.Context) error {

	n := &Node{
		pw:       upload.fs.pw,
		ID:       upload.info.Storage["NodeId"],
		ParentID: upload.info.Storage["NodeParentId"],
		Name:     upload.info.Storage["NodeName"],
	}

	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	targetPath := filepath.Join(upload.fs.conf.Root, "nodes", n.ID)

	// if target exists create new version
	if fi, err := os.Stat(targetPath); err == nil {
		// versions are stored alongside the actual file, so a rename can be efficient and does not cross storage / partition boundaries
		versionsPath := filepath.Join(upload.fs.conf.Root, "nodes", n.ID+".REV."+fi.ModTime().UTC().Format(time.RFC3339Nano))

		if err := os.Rename(targetPath, versionsPath); err != nil {
			log := appctx.GetLogger(upload.ctx)
			log.Err(err).Interface("info", upload.info).
				Str("binPath", upload.binPath).
				Str("targetPath", targetPath).
				Msg("ocisfs: could not create version")
			return err
		}
	}

	// now rename the upload to the target path
	// TODO put uploads on the same underlying storage as the destination dir?
	// TODO trigger a workflow as the final rename might eg involve antivirus scanning
	if err := os.Rename(upload.binPath, targetPath); err != nil {
		log := appctx.GetLogger(upload.ctx)
		log.Err(err).Interface("info", upload.info).
			Str("binPath", upload.binPath).
			Str("targetPath", targetPath).
			Msg("ocisfs: could not rename")
		return err
	}

	if err := xattr.Set(targetPath, "user.ocis.parentid", []byte(n.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err := xattr.Set(targetPath, "user.ocis.name", []byte(n.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}
	if u, ok := user.ContextGetUser(ctx); ok {
		if err := xattr.Set(targetPath, "user.ocis.owner.id", []byte(u.Id.OpaqueId)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner id attribute")
		}
		if err := xattr.Set(targetPath, "user.ocis.owner.idp", []byte(u.Id.Idp)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner idp attribute")
		}
	} else {
		// TODO no user in context, log as error when home enabled
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(upload.fs.conf.Root, "nodes", n.ParentID, n.Name)
	link, err := os.Readlink(childNameLink)
	if err == nil && link != "../"+n.ID {
		log.Err(err).
			Interface("info", upload.info).
			Interface("node", n).
			Str("targetPath", targetPath).
			Str("childNameLink", childNameLink).
			Str("link", link).
			Msg("ocisfs: child name link has wrong target id, repairing")

		if err := os.Remove(childNameLink); err != nil {
			return errors.Wrap(err, "ocisfs: could not remove symlink child entry")
		}
	}
	if os.IsNotExist(err) || link != "../"+n.ID {
		if err = os.Symlink("../"+n.ID, childNameLink); err != nil {
			return errors.Wrap(err, "ocisfs: could not symlink child entry")
		}
	}

	// only delete the upload if it was successfully written to the storage
	if err := os.Remove(upload.infoPath); err != nil {
		if !os.IsNotExist(err) {
			log.Err(err).Interface("info", upload.info).Msg("ocisfs: could not delete upload info")
			return err
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
