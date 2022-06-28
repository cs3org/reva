// Copyright 2018-2022 CERN
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

package upload

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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
)

// Tree is used to manage a tree hierarchy
type Tree interface {
	Setup() error

	GetMD(ctx context.Context, node *node.Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *node.Node) ([]*node.Node, error)
	// CreateHome(owner *userpb.UserId) (n *node.Node, err error)
	CreateDir(ctx context.Context, node *node.Node) (err error)
	// CreateReference(ctx context.Context, node *node.Node, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error)
	Delete(ctx context.Context, node *node.Node) (err error)
	RestoreRecycleItemFunc(ctx context.Context, spaceid, key, trashPath string, target *node.Node) (*node.Node, *node.Node, func() error, error)
	PurgeRecycleItemFunc(ctx context.Context, spaceid, key, purgePath string) (*node.Node, func() error, error)

	WriteBlob(node *node.Node, reader io.Reader) error
	ReadBlob(node *node.Node) (io.ReadCloser, error)
	DeleteBlob(node *node.Node) error

	Propagate(ctx context.Context, node *node.Node) (err error)
}

// Upload processes the upload
// it implements tus tusd.Upload interface https://tus.io/protocols/resumable-upload.html#core-protocol
// it also implements its termination extension as specified in https://tus.io/protocols/resumable-upload.html#termination
// it also implements its creation-defer-length extension as specified in https://tus.io/protocols/resumable-upload.html#creation
// it also implements its concatenation extension as specified in https://tus.io/protocols/resumable-upload.html#concatenation
type Upload struct {
	// we use a struct field on the upload as tus pkg will give us an empty context.Background
	Ctx context.Context
	// info stores the current information about the upload
	Info tusd.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
	// lu and tp needed for file operations
	lu *lookup.Lookup
	tp Tree
	// node for easy access
	node *node.Node
	// oldsize will be nil if there was no file before
	oldsize *uint64
	// Postprocessing to start postprocessing
	pp postprocessing.Postprocessing
	// TODO add logger as well?
}

// WriteChunk writes the stream from the reader to the given offset of the upload
func (upload *Upload) WriteChunk(_ context.Context, offset int64, src io.Reader) (int64, error) {
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
	if err != nil && err != io.ErrUnexpectedEOF {
		return n, err
	}

	upload.Info.Offset += n
	return n, upload.writeInfo()
}

// GetInfo returns the FileInfo
func (upload *Upload) GetInfo(_ context.Context) (tusd.FileInfo, error) {
	return upload.Info, nil
}

// GetReader returns an io.Reader for the upload
func (upload *Upload) GetReader(_ context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *Upload) FinishUpload(_ context.Context) error {
	// set lockID to context
	if upload.Info.MetaData["lockid"] != "" {
		upload.Ctx = ctxpkg.ContextSetLockID(upload.Ctx, upload.Info.MetaData["lockid"])
	}

	return upload.pp.Start()
}

// Terminate terminates the upload
func (upload *Upload) Terminate(_ context.Context) error {
	upload.cleanup(errors.New("upload terminated"))
	return nil
}

// DeclareLength updates the upload length information
func (upload *Upload) DeclareLength(_ context.Context, length int64) error {
	upload.Info.Size = length
	upload.Info.SizeIsDeferred = false
	return upload.writeInfo()
}

// ConcatUploads concatenates multiple uploads
func (upload *Upload) ConcatUploads(_ context.Context, uploads []tusd.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*Upload)

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

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *Upload) writeInfo() error {
	data, err := json.Marshal(upload.Info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// finishUpload finishes an upload and moves the file to the internal destination
func (upload *Upload) finishUpload() (err error) {
	n := upload.node
	if n == nil {
		return errors.New("need node to finish upload")
	}

	_ = xattrs.Set(upload.binPath, "user.ocis.nodestatus", "processing")

	spaceID := upload.Info.Storage["SpaceRoot"]
	targetPath := n.InternalPath()
	sublog := appctx.GetLogger(upload.Ctx).
		With().
		Interface("info", upload.Info).
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
	if upload.Info.MetaData["checksum"] != "" {
		parts := strings.SplitN(upload.Info.MetaData["checksum"], " ", 2)
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
	n.BlobID = upload.Info.ID // This can be changed to a content hash in the future when reference counting for the blobs was added

	// defer writing the checksums until the node is in place

	// if target exists create new version
	versionsPath := ""
	if fi, err := os.Stat(targetPath); err == nil && upload.oldsize != nil {
		// When the if-match header was set we need to check if the
		// etag still matches before finishing the upload.
		if ifMatch, ok := upload.Info.MetaData["if-match"]; ok {
			var targetEtag string
			targetEtag, err = node.CalculateEtag(n.ID, fi.ModTime())
			if err != nil {
				return errtypes.InternalError(err.Error())
			}
			if ifMatch != targetEtag {
				return errtypes.Aborted("etag mismatch")
			}
		}

		// FIXME move versioning to blobs ... no need to copy all the metadata! well ... it does if we want to version metadata...
		// versions are stored alongside the actual file, so a rename can be efficient and does not cross storage / partition boundaries
		versionsPath = upload.lu.InternalPath(spaceID, n.ID+node.RevisionIDDelimiter+fi.ModTime().UTC().Format(time.RFC3339Nano))

		// This move drops all metadata!!! We copy it below with CopyMetadata
		// FIXME the node must remain the same. otherwise we might restore share metadata
		if err = os.Rename(targetPath, versionsPath); err != nil {
			sublog.Err(err).
				Str("binPath", upload.binPath).
				Str("versionsPath", versionsPath).
				Msg("Decomposedfs: could not create version")
			return err
		}

		// NOTE: In case there is an existing version we have
		// - a processing flag on the version
		// - a processing flag on the binPath
		// - NO processing flag on the targetPath, as we just moved that file
		// so we remove the processing flag from version,
		_ = xattrs.Remove(versionsPath, "user.ocis.nodestatus")
		// create an empty file instead,
		_, _ = os.Create(targetPath)
		// and set the processing flag on this
		_ = xattrs.Set(targetPath, "user.ocis.nodestatus", "processing")
		// TODO: that means that there is a short amount of time when there is no targetPath
		// If clients query in exactly that moment the file will be gone from their PROPFIND
		// How can we omit this issue? How critical is it?

	}

	// upload the data to the blobstore
	file, err := os.Open(upload.binPath)
	if err != nil {
		return err
	}
	defer file.Close()
	err = upload.tp.WriteBlob(n, file)
	if err != nil {
		return errors.Wrap(err, "failed to upload file to blostore")
	}

	// now truncate the upload (the payload stays in the blobstore) and move it to the target path
	// TODO put uploads on the same underlying storage as the destination dir?
	if err = os.Truncate(upload.binPath, 0); err != nil {
		sublog.Err(err).
			Msg("Decomposedfs: could not truncate")
		return err
	}
	if err = os.Rename(upload.binPath, targetPath); err != nil {
		sublog.Error().Err(err).Msg("Decomposedfs: could not rename")
		return err
	}
	if versionsPath != "" {
		// copy grant and arbitrary metadata
		// FIXME ... now restoring an older revision might bring back a grant that was removed!
		err = xattrs.CopyMetadata(versionsPath, targetPath, func(attributeName string) bool {
			return true
			// TODO determine all attributes that must be copied, currently we just copy all and overwrite changed properties
			/*
				return strings.HasPrefix(attributeName, xattrs.GrantPrefix) || // for grants
					strings.HasPrefix(attributeName, xattrs.MetadataPrefix) || // for arbitrary metadata
					strings.HasPrefix(attributeName, xattrs.FavPrefix) || // for favorites
					strings.HasPrefix(attributeName, xattrs.SpaceNameAttr) || // for a shared file
			*/
		})
		if err != nil {
			sublog.Info().Err(err).Msg("Decomposedfs: failed to copy xattrs")
		}
	}

	// now try write all checksums
	tryWritingChecksum(&sublog, n, "sha1", sha1h)
	tryWritingChecksum(&sublog, n, "md5", md5h)
	tryWritingChecksum(&sublog, n, "adler32", adler32h)

	// who will become the owner?  the owner of the parent actually ... not the currently logged in user
	err = n.WriteAllNodeMetadata()
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentInternalPath(), n.Name)
	var link string
	link, err = os.Readlink(childNameLink)
	if err == nil && link != "../"+n.ID {
		sublog.Err(err).
			Interface("node", n).
			Str("childNameLink", childNameLink).
			Str("link", link).
			Msg("Decomposedfs: child name link has wrong target id, repairing")

		if err = os.Remove(childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not remove symlink child entry")
		}
	}
	if errors.Is(err, iofs.ErrNotExist) || link != "../"+n.ID {
		relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
		if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not symlink child entry")
		}
	}

	// only delete the upload if it was successfully written to the storage
	if err = os.Remove(upload.infoPath); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			sublog.Err(err).Msg("Decomposedfs: could not delete upload info")
			return err
		}
	}
	// use set arbitrary metadata?
	if upload.Info.MetaData["mtime"] != "" {
		err := n.SetMtime(upload.Ctx, upload.Info.MetaData["mtime"])
		if err != nil {
			sublog.Err(err).Interface("info", upload.Info).Msg("Decomposedfs: could not set mtime metadata")
			return err
		}
	}

	n.Exists = true

	return upload.tp.Propagate(upload.Ctx, n)
}

func (upload *Upload) checkHash(expected string, h hash.Hash) error {
	if expected != hex.EncodeToString(h.Sum(nil)) {
		return errtypes.ChecksumMismatch(fmt.Sprintf("invalid checksum: expected %s got %x", upload.Info.MetaData["checksum"], h.Sum(nil)))
	}
	return nil
}

// cleanup cleans up after the upload is finished
// TODO: error handling?
func (upload *Upload) cleanup(err error) {
	if upload.node != nil {
		// NOTE: this should not be part of the upload. The upload doesn't know
		// when the processing is finshed. It just cares about the actual upload
		// However, when not removing it here the testsuite will fail as it
		// can't handle processing status at the moment.
		// TODO: adjust testsuite, remove this if case and adjust PostProcessing to not wait for "assembling"
		_ = upload.node.RemoveMetadata("user.ocis.nodestatus")
	}

	if upload.node != nil && err != nil && upload.oldsize == nil {
		_ = utils.RemoveItem(upload.node.InternalPath())
	}

	_ = os.Remove(upload.binPath)
	_ = os.Remove(upload.infoPath)
}

func tryWritingChecksum(log *zerolog.Logger, n *node.Node, algo string, h hash.Hash) {
	if err := n.SetChecksum(algo, h); err != nil {
		log.Err(err).
			Str("csType", algo).
			Bytes("hash", h.Sum(nil)).
			Msg("Decomposedfs: could not write checksum")
		// this is not critical, the bytes are there so we will continue
	}
}
