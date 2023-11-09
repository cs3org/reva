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

// Package upload handles the processing of uploads.
// In general this is the lifecycle of an upload from the perspective of a storageprovider:
// 1. To start an upload a client makes a call to InitializeUpload which will return protocols and urls that he can use to append bytes to the upload.
// 2. When the client has sent all bytes the tusd handler will call a PreFinishResponseCallback which marks the end of the transfer and the start of postprocessing.
// 3. When async uploads are enabled the storageprovider emits an BytesReceived event, otherwise a FileUploaded event and the upload lifcycle ends.
// 4. During async postprocessing the uploaded bytes might be read at the upload URL to determine the outcome of the postprocessing steps
// 5. To handle async postprocessing the storageporvider has to listen to multiple events:
//   - PostprocessingFinished determines what should happen with the upload:
//   - abort - the upload is cancelled but the bytes are kept in the upload folder, eg. when antivirus scanning encounters an error
//     then what? can the admin retrigger the upload?
//   - continue - the upload is moved to its final destination (eventually being marked with pp results)
//   - delete - the file and the upload should be deleted
//   - RestartPostprocessing
//   - PostprocessingStepFinished is used to set scan data on an upload
//
// 6. The storageprovider emits an UploadReady event that can be used by eg. the search or thumbnails services to do update their metadata.
//
// There are two interesting scenarios:
// 1. Two concurrent requests try to create the same file
// 2. Two concurrent requests try to overwrite the same file
// The first step to upload a file is making an InitiateUpload call to the storageprovider via CS3. It will return an upload id that can be used to append bytes to the upload.
// With an upload id clients can append bytes to the upload.
// When all bytes have been received tusd will call PreFinishResponseCallback on the storageprovider.
// The storageprovider cannot use the tus upload metadata to persist a postprocessing status we have to store the processing status on a revision node.
// On disk the layout for a node consists of the actual node metadata and revision nodes.
// The revision nodes are used to capture the different revsions ...
// * so every uploed always creates a revision node first?
// * and in PreFinishResponseCallback we update or create? the actual node? or do we create the node in the InitiateUpload call?
// * We need to skip unfinished revisions when listing versions?
// The size diff is always calculated when updating the node
//
// ## Client considerations
// When do we propagate the etag? Currently, already when an upload is in postprocessing ... why? because we update the node when all bytes are transferred?
// Does the client expect an etag change when it uploads a file? it should not ... sync and uploads are independent last someone explained it to me
// postprocessing könnte den content ändern und damit das etag
//
// When the client finishes transferring all bytes it gets the 'future' etag of the resource which it currently stores as the etag for the file in its local db.
// When the next propfind happens before postprocessing finishes the client would see the old etag and download the old version. Then, when postprocessing causes
// the next etag change, the client will download the file it previously uploaded.
//
// For the new file scenario, the desktop client would delete the uploaded file locally, when it is not listed in the next propfind.
//
// The graph api exposes pending uploads explicitly using the pendingOperations property, which carries a pendingContentUpdate resource with a
// queuedDateTime property: Date and time the pending binary operation was queued in UTC time. Read-only.
//
// So, until clients learn to keep track of their uploads we need to return 425 when an upload is in progress ಠ_ಠ
package upload

import (
	"context"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx/manager/tus"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/lockedfile"
	tusd "github.com/tus/tusd/pkg/handler"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("github.com/cs3org/reva/pkg/storage/utils/decomposedfs/upload")
}

// CreateNewRevision will create a new revision node
func CreateNewRevision(ctx context.Context, lu *lookup.Lookup, path string, fsize uint64) (*lockedfile.File, error) {
	_, span := tracer.Start(ctx, "CreateNewRevision")
	defer span.End()

	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	// create and write lock new node metadata by parentid/name
	f, err := lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(path), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// CreateNewNode will lock the given node and try to symlink it to the parent
func CreateNewNode(ctx context.Context, lu *lookup.Lookup, n *node.Node, fsize uint64) (*lockedfile.File, error) {
	ctx, span := tracer.Start(ctx, "CreateNewNode")
	defer span.End()

	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return nil, err
	}

	// create and write lock new node metadata by parentid/name
	f, err := lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(n.InternalPath()), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// we also need to touch the actual node file here it stores the mtime of the resource
	h, err := os.OpenFile(n.InternalPath(), os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return f, err
	}
	h.Close()

	if _, err := node.CheckQuota(ctx, n.SpaceRoot, false, 0, fsize); err != nil {
		return f, err
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentPath(), n.Name)
	relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
	log := appctx.GetLogger(ctx).With().Str("childNameLink", childNameLink).Str("relativeNodePath", relativeNodePath).Logger()
	log.Info().Msg("createNewNode: creating symlink")

	if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
		log.Info().Err(err).Msg("createNewNode: symlink failed")
		if errors.Is(err, iofs.ErrExist) {
			log.Info().Err(err).Msg("createNewNode: symlink already exists")

			return f, errtypes.AlreadyExists(n.Name)
		}
		return f, errors.Wrap(err, "Decomposedfs: could not symlink child entry")
	}
	log.Info().Msg("createNewNode: symlink created")

	return f, nil
}

// CreateRevision will create the target node for the Upload
// - if the node does not exist it is created and assigned an id, no blob id?
// - then always write out a revision node
// - when postprocessing finishes copy metadata to node and replace latest revision node with previous blob info. if blobid is empty delete previous revision completely?
func CreateRevision(ctx context.Context, lu *lookup.Lookup, info tusd.FileInfo, attrs node.Attributes) (*node.Node, error) {
	ctx, span := tracer.Start(ctx, "CreateRevision")
	defer span.End()
	log := appctx.GetLogger(ctx).With().Str("uploadID", info.ID).Logger()

	// check lock
	if info.MetaData[tus.CS3Prefix+"lockid"] != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, info.MetaData[tus.CS3Prefix+"lockid"])
	}

	var err error

	// FIXME should uploads fail if they try to overwrite an existing file?
	// but if the webdav overwrite header is set ... two concurrent requests might each create a node with a different id ... -> same problem
	// two concurrent requests that would create a new node would return different ids ...
	// what if we generate an id based on the parent id and the filename?
	// - no, then renaming the file and recreating a node with the provious name would generate the same id
	// -> we have to create the node on initialize upload with processing true?

	var n *node.Node
	var nodeHandle *lockedfile.File
	if info.MetaData[tus.CS3Prefix+"NodeId"] == "" {
		// we need to check if the node exists via parentid & child name
		p, err := node.ReadNode(ctx, lu, info.MetaData[tus.CS3Prefix+"SpaceRoot"], info.MetaData[tus.CS3Prefix+"NodeParentId"], false, nil, true)
		if err != nil {
			log.Error().Err(err).Msg("could not read parent node")
			return nil, err
		}
		if !p.Exists {
			return nil, errtypes.PreconditionFailed("parent does not exist")
		}
		n, err = p.Child(ctx, info.MetaData[tus.CS3Prefix+"filename"])
		if err != nil {
			log.Error().Err(err).Msg("could not read parent node")
			return nil, err
		}
		if !n.Exists {
			n.ID = uuid.New().String()
			nodeHandle, err = initNewNode(ctx, lu, info, n)
			if err != nil {
				log.Error().Err(err).Msg("could not init new node")
				return nil, err
			}
			log.Info().Str("lockfile", nodeHandle.Name()).Msg("got lock file from initNewNode")
		} else {
			nodeHandle, err = openExistingNode(ctx, lu, n)
			if err != nil {
				log.Error().Err(err).Msg("could not open existing node")
				return nil, err
			}
			log.Info().Str("lockfile", nodeHandle.Name()).Msg("got lock file from openExistingNode")
		}
	}

	if nodeHandle == nil {
		n, err = node.ReadNode(ctx, lu, info.MetaData[tus.CS3Prefix+"SpaceRoot"], info.MetaData[tus.CS3Prefix+"NodeId"], false, nil, true)
		if err != nil {
			log.Error().Err(err).Msg("could not read parent node")
			return nil, err
		}
		nodeHandle, err = openExistingNode(ctx, lu, n)
		if err != nil {
			log.Error().Err(err).Msg("could not open existing node")
			return nil, err
		}
		log.Info().Str("lockfile", nodeHandle.Name()).Msg("got lock file from openExistingNode")
	}
	defer func() {
		if nodeHandle == nil {
			return
		}
		if err := nodeHandle.Close(); err != nil {
			log.Error().Err(err).Str("nodeid", n.ID).Str("parentid", n.ParentID).Msg("could not close lock")
		}
	}()

	err = validateRequest(ctx, info, n)
	if err != nil {
		return nil, err
	}

	newBlobID := uuid.New().String()

	// set processing status of node
	nodeAttrs := node.Attributes{}
	// store new Blobid and Blobsize in node
	// nodeAttrs.SetString(prefixes.BlobIDAttr, newBlobID) // BlobID is checked when removing a revision to decide if we also need to delete the node
	// hm ... check if any other revisions are still available?
	nodeAttrs.SetInt64(prefixes.BlobsizeAttr, info.Size) // FIXME ... argh now the propagation needs to revert the size diff propagation again
	nodeAttrs.SetString(prefixes.StatusPrefix, node.ProcessingStatus+info.ID)
	err = n.SetXattrsWithContext(ctx, nodeAttrs, false)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	revisionNode, err := n.ReadRevision(ctx, info.MetaData[tus.CS3Prefix+"RevisionTime"])
	if err != nil {
		return nil, err
	}

	var revisionHandle *lockedfile.File
	revisionHandle, err = createRevisionNode(ctx, lu, revisionNode)
	defer func() {
		if revisionHandle == nil {
			return
		}
		if err := revisionHandle.Close(); err != nil {
			log.Error().Err(err).Str("nodeid", revisionNode.ID).Str("parentid", revisionNode.ParentID).Msg("could not close lock")
		}
	}()
	if err != nil {
		return nil, err
	}

	// set upload related metadata
	if info.MetaData[tus.TusPrefix+"mtime"] == "" {
		attrs.SetString(prefixes.MTimeAttr, info.MetaData[tus.CS3Prefix+"RevisionTime"])
	} else {
		// overwrite mtime if requested
		mtime, err := utils.MTimeToTime(info.MetaData[tus.TusPrefix+"mtime"])
		if err != nil {
			return nil, err
		}
		attrs.SetString(prefixes.MTimeAttr, mtime.UTC().Format(time.RFC3339Nano))
	}
	attrs.SetString(prefixes.BlobIDAttr, newBlobID)
	attrs.SetInt64(prefixes.BlobsizeAttr, info.Size)
	// TODO we should persist all versions as writes with ranges and the blobid in the node metadata
	// attrs.SetString(prefixes.StatusPrefix, node.ProcessingStatus+info.ID)

	err = revisionNode.SetXattrsWithContext(ctx, attrs, false)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	return n, nil
}

func validateRequest(ctx context.Context, info tusd.FileInfo, n *node.Node) error {
	if err := n.CheckLock(ctx); err != nil {
		return err
	}

	if _, err := node.CheckQuota(ctx, n.SpaceRoot, true, uint64(n.Blobsize), uint64(info.Size)); err != nil {
		return err
	}

	mtime, err := n.GetMTime(ctx)
	if err != nil {
		return err
	}
	currentEtag, err := node.CalculateEtag(n, mtime)
	if err != nil {
		return err
	}

	// When the if-match header was set we need to check if the
	// etag still matches before finishing the upload.
	if ifMatch, ok := info.MetaData[tus.CS3Prefix+"if-match"]; ok {
		if ifMatch != currentEtag {
			return errtypes.Aborted("etag mismatch")
		}
	}

	// When the if-none-match header was set we need to check if any of the
	// etags matches before finishing the upload.
	if ifNoneMatch, ok := info.MetaData[tus.CS3Prefix+"if-none-match"]; ok {
		if ifNoneMatch == "*" {
			return errtypes.Aborted("etag mismatch, resource exists")
		}
		for _, ifNoneMatchTag := range strings.Split(ifNoneMatch, ",") {
			if ifNoneMatchTag == currentEtag {
				return errtypes.Aborted("etag mismatch")
			}
		}
	}

	// When the if-unmodified-since header was set we need to check if the
	// etag still matches before finishing the upload.
	if ifUnmodifiedSince, ok := info.MetaData[tus.CS3Prefix+"if-unmodified-since"]; ok {
		if err != nil {
			return errtypes.InternalError(fmt.Sprintf("failed to read mtime of node: %s", err))
		}
		ifUnmodifiedSince, err := time.Parse(time.RFC3339Nano, ifUnmodifiedSince)
		if err != nil {
			return errtypes.InternalError(fmt.Sprintf("failed to parse if-unmodified-since time: %s", err))
		}

		if mtime.After(ifUnmodifiedSince) {
			return errtypes.Aborted("if-unmodified-since mismatch")
		}
	}
	return nil
}

func openExistingNode(ctx context.Context, lu *lookup.Lookup, n *node.Node) (*lockedfile.File, error) {
	// create and read lock existing node metadata
	return lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(n.InternalPath()), os.O_RDONLY, 0600)
}
func initNewNode(ctx context.Context, lu *lookup.Lookup, info tusd.FileInfo, n *node.Node) (*lockedfile.File, error) {
	nodePath := n.InternalPath()
	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(nodePath), 0700); err != nil {
		return nil, err
	}

	// create and write lock new node metadata
	f, err := lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(nodePath), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// FIXME if this is removed links to files will be dangling, causing subsequest stats to files to fail
	// we also need to touch the actual node file here it stores the mtime of the resource
	h, err := os.OpenFile(nodePath, os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return f, err
	}
	h.Close()

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentPath(), n.Name)
	relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
	log := appctx.GetLogger(ctx).With().Str("childNameLink", childNameLink).Str("relativeNodePath", relativeNodePath).Logger()
	log.Info().Msg("initNewNode: creating symlink")

	if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
		log.Info().Err(err).Msg("initNewNode: symlink failed")
		if errors.Is(err, iofs.ErrExist) {
			log.Info().Err(err).Msg("initNewNode: symlink already exists")
			return f, errtypes.AlreadyExists(n.Name)
		}
		return f, errors.Wrap(err, "Decomposedfs: could not symlink child entry")
	}
	log.Info().Msg("initNewNode: symlink created")

	attrs := node.Attributes{}
	attrs.SetInt64(prefixes.TypeAttr, int64(provider.ResourceType_RESOURCE_TYPE_FILE))
	attrs.SetString(prefixes.ParentidAttr, n.ParentID)
	attrs.SetString(prefixes.NameAttr, n.Name)
	attrs.SetString(prefixes.MTimeAttr, info.MetaData[tus.CS3Prefix+"RevisionTime"])

	// here we set the status the first time.
	attrs.SetString(prefixes.StatusPrefix, node.ProcessingStatus+info.ID)

	// update node metadata with basic metadata
	err = n.SetXattrsWithContext(ctx, attrs, false)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}
	return f, nil
}

func createRevisionNode(ctx context.Context, lu *lookup.Lookup, revisionNode *node.Node) (*lockedfile.File, error) {
	revisionPath := revisionNode.InternalPath()
	// write lock existing node before reading any metadata
	f, err := lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(revisionPath), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// FIXME if this is removed listing revisions breaks because it globs the dir but then filters all metadata files
	// we also need to touch the a vorsions node here to list revisions
	h, err := os.OpenFile(revisionPath, os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return f, err
	}
	h.Close()
	return f, nil
}

func SetNodeToRevision(ctx context.Context, lu *lookup.Lookup, n *node.Node, revision string) (int64, error) {

	nodePath := n.InternalPath()
	// lock existing node metadata
	f, err := lockedfile.OpenFile(lu.MetadataBackend().LockfilePath(nodePath), os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	// read nodes

	n, err = node.ReadNode(ctx, lu, n.SpaceID, n.ID, false, n.SpaceRoot, true)
	if err != nil {
		return 0, err
	}

	revisionNode, err := n.ReadRevision(ctx, revision)
	if err != nil {
		return 0, err
	}

	sizeDiff := revisionNode.Blobsize - n.Blobsize

	n.BlobID = revisionNode.BlobID
	n.Blobsize = revisionNode.Blobsize

	revisionAttrs, err := revisionNode.Xattrs(ctx)
	if err != nil {
		return 0, err
	}
	attrs := node.Attributes{}
	attrs.SetString(prefixes.BlobIDAttr, revisionNode.BlobID)
	attrs.SetInt64(prefixes.BlobsizeAttr, revisionNode.Blobsize)
	attrs[prefixes.MTimeAttr] = revisionAttrs[prefixes.MTimeAttr]

	// copy checksums TODO we need to make sure ALL old checksums are wiped
	for k, v := range revisionAttrs {
		if strings.HasPrefix(k, prefixes.ChecksumPrefix) {
			attrs[k] = v
		}
	}

	err = n.SetXattrsWithContext(ctx, attrs, false)
	if err != nil {
		return 0, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	return sizeDiff, nil
}

func ReadNode(ctx context.Context, lu *lookup.Lookup, info tusd.FileInfo) (*node.Node, error) {
	var n *node.Node
	var err error
	if info.MetaData[tus.CS3Prefix+"NodeId"] == "" {
		p, err := node.ReadNode(ctx, lu, info.MetaData[tus.CS3Prefix+"SpaceRoot"], info.MetaData[tus.CS3Prefix+"NodeParentId"], false, nil, true)
		if err != nil {
			return nil, err
		}
		n, err = p.Child(ctx, info.MetaData[tus.CS3Prefix+"filename"])
		if err != nil {
			return nil, err
		}
	} else {
		n, err = node.ReadNode(ctx, lu, info.MetaData[tus.CS3Prefix+"SpaceRoot"], info.MetaData[tus.CS3Prefix+"NodeId"], false, nil, true)
		if err != nil {
			return nil, err
		}
	}
	return n, nil
}

// Cleanup cleans the upload
func Cleanup(ctx context.Context, lu *lookup.Lookup, n *node.Node, info tusd.FileInfo, failure bool) {
	ctx, span := tracer.Start(ctx, "Cleanup")
	defer span.End()

	if n != nil { // node can be nil when there was an error before it was created (eg. checksum-mismatch)
		if failure {
			removeRevision(ctx, lu, n, info.MetaData[tus.CS3Prefix+"RevisionTime"])
		}
		// unset processing status
		if err := n.UnmarkProcessing(ctx, info.ID); err != nil {
			log := appctx.GetLogger(ctx)
			log.Info().Str("path", n.InternalPath()).Err(err).Msg("unmarking processing failed")
		}
	}
}

// removeRevision cleans up after the upload is finished
func removeRevision(ctx context.Context, lu *lookup.Lookup, n *node.Node, revision string) {
	log := appctx.GetLogger(ctx)
	nodePath := n.InternalPath()
	revisionPath := nodePath + node.RevisionIDDelimiter + revision
	// remove revision
	if err := utils.RemoveItem(revisionPath); err != nil {
		log.Info().Str("path", revisionPath).Err(err).Msg("removing revision failed")
	}
	// purge revision metadata to clean up cache
	if err := lu.MetadataBackend().Purge(revisionPath); err != nil {
		log.Info().Str("path", revisionPath).Err(err).Msg("purging revision metadata failed")
	}

	if n.BlobID == "" { // FIXME ... this is brittle
		// no old version was present - remove child entry symlink from directory
		src := filepath.Join(n.ParentPath(), n.Name)
		if err := os.Remove(src); err != nil {
			log.Info().Str("path", n.ParentPath()).Err(err).Msg("removing node from parent failed")
		}

		// delete node
		if err := utils.RemoveItem(nodePath); err != nil {
			log.Info().Str("path", nodePath).Err(err).Msg("removing node failed")
		}

		// purge node metadata to clean up cache
		if err := lu.MetadataBackend().Purge(nodePath); err != nil {
			log.Info().Str("path", nodePath).Err(err).Msg("purging node metadata failed")
		}
	}
}

// Finalize finalizes the upload (eg moves the file to the internal destination)
func Finalize(ctx context.Context, blobstore tree.Blobstore, info tusd.FileInfo, n *node.Node) error {
	_, span := tracer.Start(ctx, "Finalize")
	defer span.End()

	rn, err := n.ReadRevision(ctx, info.MetaData[tus.CS3Prefix+"RevisionTime"])
	if err != nil {
		return errors.Wrap(err, "failed to read revision")
	}
	if mover, ok := blobstore.(tree.BlobstoreMover); ok {
		err = mover.MoveBlob(rn, "", info.Storage["Bucket"], info.Storage["Key"])
		switch err {
		case nil:
			return nil
		case tree.ErrBlobstoreCannotMove:
			// fallback below
		default:
			return err
		}
	}

	// upload the data to the blobstore
	_, subspan := tracer.Start(ctx, "WriteBlob")
	err = blobstore.Upload(rn, info.Storage["Path"]) // FIXME where do we read from
	subspan.End()
	if err != nil {
		return errors.Wrap(err, "failed to upload file to blobstore")
	}

	// FIXME use a reader
	return nil
}
