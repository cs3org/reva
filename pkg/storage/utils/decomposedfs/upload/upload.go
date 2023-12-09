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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("github.com/cs3org/reva/pkg/storage/utils/decomposedfs/upload")
}

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

	WriteBlob(node *node.Node, binPath string) error
	ReadBlob(node *node.Node) (io.ReadCloser, error)
	DeleteBlob(node *node.Node) error

	StreamBlob(ctx context.Context, spaceid, blobid string, offset, objectSize int64, reader io.Reader, userMetadata map[string]string) error
	BlobReader(ctx context.Context, spaceid, blobid string, offset, objectSize int64) (io.ReadCloser, error)

	Propagate(ctx context.Context, node *node.Node, sizeDiff int64) (err error)
}

const (
	OptimalPartSize = int64(16 * 1024 * 1024)
)

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
	// node for easy access
	Node *node.Node
	// SizeDiff size difference between new and old file version
	SizeDiff int64
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
	// lu and tp needed for file operations
	lu *lookup.Lookup
	tp Tree
	// versionsPath will be empty if there was no file before
	versionsPath string
	// and a logger as well
	log zerolog.Logger
	// publisher used to publish events
	pub events.Publisher
	// async determines if uploads shoud be done asynchronously
	async bool
	// tknopts hold token signing information
	tknopts options.TokenOptions

	// blob offsets
	//BlobOffsets []int64

	// hash states
	//SHA1Enc    []byte
	//MD5Enc     []byte
	//ADLER32Enc []byte
}

func buildUpload(ctx context.Context, info tusd.FileInfo, binPath string, infoPath string, lu *lookup.Lookup, tp Tree, pub events.Publisher, async bool, tknopts options.TokenOptions) *Upload {
	return &Upload{
		Info:     info,
		binPath:  binPath,
		infoPath: infoPath,
		lu:       lu,
		tp:       tp,
		Ctx:      ctx,
		pub:      pub,
		async:    async,
		tknopts:  tknopts,
		log: appctx.GetLogger(ctx).
			With().
			Interface("info", info).
			Str("binPath", binPath).
			Logger(),
	}
}

// Cleanup cleans the upload
func Cleanup(upload *Upload, failure bool, keepUpload bool) {
	ctx, span := tracer.Start(upload.Ctx, "Cleanup")
	defer span.End()
	upload.cleanup(failure, !keepUpload, !keepUpload)

	// unset processing status
	if upload.Node != nil { // node can be nil when there was an error before it was created (eg. checksum-mismatch)
		if err := upload.Node.UnmarkProcessing(ctx, upload.Info.ID); err != nil {
			upload.log.Info().Str("path", upload.Node.InternalPath()).Err(err).Msg("unmarking processing failed")
		}
	}
}

// WriteChunk writes the stream from the reader to the given offset of the upload
func (upload *Upload) WriteChunk(_ context.Context, offset int64, src io.Reader) (int64, error) {
	ctx, span := tracer.Start(upload.Ctx, "WriteChunk")
	defer span.End()

	partsChan := make(chan *bytes.Buffer)
	doneChan := make(chan struct{})
	defer close(doneChan)

	pp := partProducer{
		done:  doneChan,
		parts: partsChan,
		r:     src,
	}
	go pp.produce(OptimalPartSize)

	// The for loop will update the offset and blob offsets of the upload that we want to remember, so we always try to write the info
	defer upload.writeInfo()
	// TODO this adds an additional write in comparison to the filestore backend,
	// because the filestore uses the binpart length on disk for the offset.
	// we could try to store the upload session in s3? but it really needs locking, then.

	var writtenBytes int64
	blobOffsets := []string{}
	for part := range partsChan {
		blobOffsets = append(blobOffsets, strconv.FormatInt(offset, 10))
		upload.Info.Storage["BlobOffsets"] = strings.Join(blobOffsets, ",")

		sha1h := sha1.New()
		md5h := md5.New()
		adler32h := adler32.New()

		r1 := io.TeeReader(part, sha1h)
		r2 := io.TeeReader(r1, md5h)
		r3 := io.TeeReader(r2, adler32h)

		partsize := int64(part.Len())
		_, subspan := tracer.Start(ctx, "tp.StreamBlob")
		err := upload.tp.StreamBlob(upload.Ctx, upload.Info.Storage["SpaceRoot"], upload.Info.ID, offset, partsize, r3, map[string]string{})
		subspan.End()
		if err != nil {
			// TODO can we seek to the beginning of the buffer and try again? or write to local disk?
			return 0, err
		}
		// free buffer
		part.Reset()

		sha1hm, _ := sha1h.(encoding.BinaryMarshaler)
		sha1enc, err := sha1hm.MarshalBinary()
		if err != nil {
			return 0, err
		}
		upload.Info.Storage["SHA1Enc"] = hex.EncodeToString(sha1enc)

		md5hm, _ := md5h.(encoding.BinaryMarshaler)
		md5enc, err := md5hm.MarshalBinary()
		if err != nil {
			return 0, err
		}
		upload.Info.Storage["MD5Enc"] = hex.EncodeToString(md5enc)

		adler32hm, _ := adler32h.(encoding.BinaryMarshaler)
		adler32enc, err := adler32hm.MarshalBinary()
		if err != nil {
			return 0, err
		}
		upload.Info.Storage["ADLER32Enc"] = hex.EncodeToString(adler32enc)

		upload.Info.Offset += partsize
		writtenBytes += partsize
	}

	return writtenBytes, nil
}

// GetInfo returns the FileInfo
func (upload *Upload) GetInfo(_ context.Context) (tusd.FileInfo, error) {
	return upload.Info, nil
}

// GetReader returns an io.Reader for the upload
func (upload *Upload) GetReader(_ context.Context) (io.Reader, error) {
	_, span := tracer.Start(upload.Ctx, "GetReader")
	defer span.End()
	blobOffsets := strings.Split(upload.Info.Storage["BlobOffsets"], ",")
	blobparts := len(blobOffsets)
	readers := make([]io.Reader, 0, blobparts)
	for i, offsetStr := range blobOffsets {
		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			return nil, err
		}
		var partSize int64
		if i < blobparts && blobparts > 1 {
			partSize = OptimalPartSize
		} else {
			partSize = upload.Info.Size - offset
		}
		blobReader, err := upload.tp.BlobReader(upload.Ctx, upload.Info.Storage["SpaceRoot"], upload.Info.ID, offset, partSize)
		if err != nil {
			return nil, err
		}
		readers = append(readers, CloseOnEOFReader(blobReader))
	}
	return io.MultiReader(readers...), nil
}

type closeOnEOFReader struct {
	rc io.ReadCloser
}

func (closer closeOnEOFReader) Read(p []byte) (int, error) {
	n, err := closer.rc.Read(p)
	if err == io.EOF {
		closer.rc.Close()
	}
	return n, err
}
func CloseOnEOFReader(rc io.ReadCloser) io.Reader {
	return closeOnEOFReader{rc: rc}
}

// FinishUpload finishes an upload and moves the file to the internal destination
func (upload *Upload) FinishUpload(_ context.Context) error {
	ctx, span := tracer.Start(upload.Ctx, "FinishUpload")
	defer span.End()
	// set lockID to context
	if upload.Info.MetaData["lockid"] != "" {
		upload.Ctx = ctxpkg.ContextSetLockID(upload.Ctx, upload.Info.MetaData["lockid"])
	}

	log := appctx.GetLogger(upload.Ctx)

	// unmarshal the hashes of the written blob parts
	sha1Enc, err := hex.DecodeString(upload.Info.Storage["SHA1Enc"])
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}
	sha1h := sha1.New()
	sha1hu, _ := sha1h.(encoding.BinaryUnmarshaler)
	err = sha1hu.UnmarshalBinary(sha1Enc)
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}

	md5h := md5.New()
	md5hu, _ := md5h.(encoding.BinaryUnmarshaler)
	md5enc, err := hex.DecodeString(upload.Info.Storage["MD5Enc"])
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}
	err = md5hu.UnmarshalBinary(md5enc)
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}

	adler32h := adler32.New()
	adler32u, _ := adler32h.(encoding.BinaryUnmarshaler)
	adler32enc, err := hex.DecodeString(upload.Info.Storage["ADLER32Enc"])
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}
	err = adler32u.UnmarshalBinary(adler32enc)
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}

	// compare if they match the sent checksum
	// TODO the tus checksum extension would do this on every chunk, but I currently don't see an easy way to pass in the requested checksum. for now we do it in FinishUpload which is also called for chunked uploads
	if upload.Info.MetaData["checksum"] != "" {
		var err error
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
			Cleanup(upload, true, false)
			return err
		}
	}

	// update checksums
	attrs := node.Attributes{
		prefixes.ChecksumPrefix + "sha1":    sha1h.Sum(nil),
		prefixes.ChecksumPrefix + "md5":     md5h.Sum(nil),
		prefixes.ChecksumPrefix + "adler32": adler32h.Sum(nil),
	}

	n, err := CreateNodeForUpload(upload, attrs)
	if err != nil {
		Cleanup(upload, true, false)
		return err
	}

	upload.Node = n

	if upload.pub != nil {
		u, _ := ctxpkg.ContextGetUser(upload.Ctx)
		s, err := upload.URL(upload.Ctx)
		if err != nil {
			return err
		}

		if err := events.Publish(ctx, upload.pub, events.BytesReceived{
			UploadID:      upload.Info.ID,
			URL:           s,
			SpaceOwner:    n.SpaceOwnerOrManager(upload.Ctx),
			ExecutingUser: u,
			ResourceID:    &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID},
			Filename:      upload.Info.Storage["NodeName"],
			Filesize:      uint64(upload.Info.Size),
		}); err != nil {
			return err
		}
	}

	if !upload.async {
		// handle postprocessing synchronously
		err = upload.Finalize()
		Cleanup(upload, err != nil, false)
		if err != nil {
			log.Error().Err(err).Msg("failed to upload")
			return err
		}
	}

	return upload.tp.Propagate(upload.Ctx, n, upload.SizeDiff)
}

// Terminate terminates the upload
func (upload *Upload) Terminate(_ context.Context) error {
	upload.cleanup(true, true, true)
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
	_, span := tracer.Start(upload.Ctx, "writeInfo")
	defer span.End()
	data, err := json.Marshal(upload.Info) // FIXME write all upload session metadada
	if err != nil {
		return err
	}
	return os.WriteFile(upload.infoPath, data, defaultFilePerm)
}

// Finalize finalizes the upload (eg moves the file to the internal destination)
func (upload *Upload) Finalize() (err error) {
	/*
		ctx, span := tracer.Start(upload.Ctx, "Finalize")
		defer span.End()
		n := upload.Node
		if n == nil {
			var err error
			n, err = node.ReadNode(ctx, upload.lu, upload.Info.Storage["SpaceRoot"], upload.Info.Storage["NodeId"], false, nil, false)
			if err != nil {
				return err
			}
			upload.Node = n
		}

		// upload the data to the blobstore
		_, subspan := tracer.Start(ctx, "WriteBlob")
		err = upload.tp.WriteBlob(n, upload.binPath)
		subspan.End()
		if err != nil {
			return errors.Wrap(err, "failed to upload file to blobstore")
		}

	*/
	return nil
}

func (upload *Upload) checkHash(expected string, h hash.Hash) error {
	if expected != hex.EncodeToString(h.Sum(nil)) {
		return errtypes.ChecksumMismatch(fmt.Sprintf("invalid checksum: expected %s got %x", upload.Info.MetaData["checksum"], h.Sum(nil)))
	}
	return nil
}

// cleanup cleans up after the upload is finished
func (upload *Upload) cleanup(cleanNode, cleanBin, cleanInfo bool) {
	if cleanNode && upload.Node != nil {
		switch p := upload.versionsPath; p {
		case "":
			// remove node
			if err := utils.RemoveItem(upload.Node.InternalPath()); err != nil {
				upload.log.Info().Str("path", upload.Node.InternalPath()).Err(err).Msg("removing node failed")
			}

			// no old version was present - remove child entry
			src := filepath.Join(upload.Node.ParentPath(), upload.Node.Name)
			if err := os.Remove(src); err != nil {
				upload.log.Info().Str("path", upload.Node.ParentPath()).Err(err).Msg("removing node from parent failed")
			}

			// remove node from upload as it no longer exists
			upload.Node = nil
		default:

			if err := upload.lu.CopyMetadata(upload.Ctx, p, upload.Node.InternalPath(), func(attributeName string, value []byte) (newValue []byte, copy bool) {
				return value, strings.HasPrefix(attributeName, prefixes.ChecksumPrefix) ||
					attributeName == prefixes.TypeAttr ||
					attributeName == prefixes.BlobIDAttr ||
					attributeName == prefixes.BlobsizeAttr ||
					attributeName == prefixes.BlobOffsetsAttr ||
					attributeName == prefixes.MTimeAttr
			}, true); err != nil {
				upload.log.Info().Str("versionpath", p).Str("nodepath", upload.Node.InternalPath()).Err(err).Msg("renaming version node failed")
			}

			if err := os.RemoveAll(p); err != nil {
				upload.log.Info().Str("versionpath", p).Str("nodepath", upload.Node.InternalPath()).Err(err).Msg("error removing version")
			}

		}
	}

	if cleanBin {
		if err := os.Remove(upload.binPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			upload.log.Error().Str("path", upload.binPath).Err(err).Msg("removing upload failed")
		}
	}

	if cleanInfo {
		if err := os.Remove(upload.infoPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			upload.log.Error().Str("path", upload.infoPath).Err(err).Msg("removing upload info failed")
		}
	}
}

// URL returns a url to download an upload
func (upload *Upload) URL(_ context.Context) (string, error) {
	type transferClaims struct {
		jwt.StandardClaims
		Target string `json:"target"`
	}

	u := joinurl(upload.tknopts.DownloadEndpoint, "tus/", upload.Info.ID)
	ttl := time.Duration(upload.tknopts.TransferExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		Target: u,
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(upload.tknopts.TransferSharedSecret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return joinurl(upload.tknopts.DataGatewayEndpoint, tkn), nil
}

// replace with url.JoinPath after switching to go1.19
func joinurl(paths ...string) string {
	var s strings.Builder
	l := len(paths)
	for i, p := range paths {
		s.WriteString(p)
		if !strings.HasSuffix(p, "/") && i != l-1 {
			s.WriteString("/")
		}
	}

	return s.String()
}
