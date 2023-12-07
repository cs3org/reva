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
	stderrors "errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/tus"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/lockedfile"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
}

// New returns a new processing instance
func New(ctx context.Context, session tus.Session, lu *lookup.Lookup, tp Tree, p PermissionsChecker, fsRoot string, pub events.Publisher, async bool, tknopts options.TokenOptions) (upload *Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("session", session).Msg("Decomposedfs: NewUpload")

	if session.Filename == "" {
		return nil, errors.New("Decomposedfs: missing filename in metadata")
	}
	if session.Dir == "" {
		return nil, errors.New("Decomposedfs: missing dir in metadata")
	}

	n, err := lu.NodeFromSpaceID(ctx, session.SpaceRoot)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error getting space root node")
	}

	n, err = lookupNode(ctx, n, filepath.Join(session.Dir, session.Filename), lu)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error walking path")
	}

	log.Debug().Interface("session", session).Interface("node", n).Msg("Decomposedfs: resolved filename")

	// the parent owner will become the new owner
	parent, perr := n.Parent(ctx)
	if perr != nil {
		return nil, errors.Wrap(perr, "Decomposedfs: error getting parent "+n.ParentID)
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
		checkNode = parent
		path, _ = storagespace.FormatReference(&provider.Reference{ResourceId: &provider.ResourceId{
			SpaceId:  checkNode.SpaceID,
			OpaqueId: checkNode.ID,
		}, Path: n.Name})
	}
	rp, err := p.AssemblePermissions(ctx, checkNode)
	switch {
	case err != nil:
		return nil, err
	case !rp.InitiateFileUpload:
		return nil, errtypes.PermissionDenied(path)
	}

	// are we trying to overwriting a folder with a file?
	if n.Exists && n.IsDir(ctx) {
		return nil, errtypes.PreconditionFailed("resource is not a file")
	}

	// check lock
	if session.LockID != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, session.LockID)
	}
	if err := n.CheckLock(ctx); err != nil {
		return nil, err
	}

	session.ID = uuid.New().String()

	usr := ctxpkg.ContextMustGetUser(ctx)

	// fill future node info
	if n.Exists {
		if session.HeaderIfNoneMatch == "*" {
			return nil, errtypes.Aborted(fmt.Sprintf("parent %s already has a child %s, id %s", n.ParentID, n.Name, n.ID))
		}
		session.NodeID = n.ID
		session.NodeExists = true
	} else {
		session.NodeID = uuid.New().String()
	}
	session.NodeParentID = n.ParentID
	// TODO store userid struct
	session.ExecutantID = usr.Id.OpaqueId
	session.ExecutantIdp = usr.Id.Idp
	session.ExecutantType = utils.UserTypeToString(usr.Id.Type)
	session.ExecutantUserName = usr.Username

	session.LogLevel = log.GetLevel().String()

	// Create binary file in the upload folder with no content
	// It will be used when determining the current offset of an upload
	log.Debug().Interface("session", session).Msg("Decomposedfs: built session info")
	binPath := filepath.Join(fsRoot, "uploads", session.ID)
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	u := buildUpload(ctx, session, binPath, lu, tp, pub, async, tknopts)

	err = session.Persist(ctx)
	if err != nil {
		return nil, err
	}

	return u, nil
}

// Get returns the Upload for the given upload id
func Get(ctx context.Context, id string, lu *lookup.Lookup, tp Tree, fsRoot string, pub events.Publisher, async bool, tknopts options.TokenOptions) (*Upload, error) {
	session, err := tus.ReadSession(ctx, fsRoot, id)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = tusd.ErrNotFound
		}
		return nil, err
	}

	binPath := filepath.Join(fsRoot, "uploads", session.ID)
	stat, err := os.Stat(binPath)
	if err != nil {
		return nil, err
	}

	session.Offset = stat.Size()

	u := &userpb.User{
		Id: &userpb.UserId{
			Idp:      session.ExecutantIdp,
			OpaqueId: session.ExecutantID,
			Type:     utils.UserTypeMap(session.ExecutantType),
		},
		Username: session.ExecutantUserName,
	}

	ctx = ctxpkg.ContextSetUser(ctx, u)

	// restore logger from file info
	log, err := logger.FromConfig(&logger.LogConf{
		Output: "stderr", // TODO use config from decomposedfs
		Mode:   "json",   // TODO use config from decomposedfs
		Level:  session.LogLevel,
	})
	if err != nil {
		return nil, err
	}
	sub := log.With().Int("pid", os.Getpid()).Logger()
	ctx = appctx.WithLogger(ctx, &sub)

	// TODO store and add traceid in file info

	up := buildUpload(ctx, session, binPath, lu, tp, pub, async, tknopts)
	return up, nil
}

// CreateNodeForUpload will create the target node for the Upload
func CreateNodeForUpload(upload *Upload, initAttrs node.Attributes) (*node.Node, error) {
	ctx, span := tracer.Start(upload.Ctx, "CreateNodeForUpload")
	defer span.End()
	_, subspan := tracer.Start(ctx, "os.Stat")
	fi, err := os.Stat(upload.binPath)
	subspan.End()
	if err != nil {
		return nil, err
	}

	fsize := fi.Size()
	n := node.New(
		upload.Session.SpaceRoot,
		upload.Session.NodeID,
		upload.Session.NodeParentID,
		upload.Session.Filename,
		fsize,
		upload.Session.ID,
		provider.ResourceType_RESOURCE_TYPE_FILE,
		nil,
		upload.lu,
	)
	n.SpaceRoot, err = node.ReadNode(ctx, upload.lu, upload.Session.SpaceRoot, upload.Session.SpaceRoot, false, nil, false)
	if err != nil {
		return nil, err
	}

	// check lock
	if err := n.CheckLock(ctx); err != nil {
		return nil, err
	}

	var f *lockedfile.File
	if upload.Session.NodeExists {
		f, err = updateExistingNode(upload, n, upload.Session.SpaceRoot, uint64(fsize))
		if f != nil {
			appctx.GetLogger(upload.Ctx).Info().Str("lockfile", f.Name()).Interface("err", err).Msg("got lock file from updateExistingNode")
		}
	} else {
		f, err = initNewNode(upload, n, uint64(fsize))
		if f != nil {
			appctx.GetLogger(upload.Ctx).Info().Str("lockfile", f.Name()).Interface("err", err).Msg("got lock file from initNewNode")
		}
	}
	defer func() {
		if f == nil {
			return
		}
		if err := f.Close(); err != nil {
			appctx.GetLogger(upload.Ctx).Error().Err(err).Str("nodeid", n.ID).Str("parentid", n.ParentID).Msg("could not close lock")
		}
	}()
	if err != nil {
		return nil, err
	}

	mtime := time.Now()
	if upload.Session.MetaData["mtime"] != "" {
		// overwrite mtime if requested
		mtime, err = utils.MTimeToTime(upload.Session.MetaData["mtime"])
		if err != nil {
			return nil, err
		}
	}

	// overwrite technical information
	initAttrs.SetString(prefixes.MTimeAttr, mtime.UTC().Format(time.RFC3339Nano))
	initAttrs.SetInt64(prefixes.TypeAttr, int64(provider.ResourceType_RESOURCE_TYPE_FILE))
	initAttrs.SetString(prefixes.ParentidAttr, n.ParentID)
	initAttrs.SetString(prefixes.NameAttr, n.Name)
	initAttrs.SetString(prefixes.BlobIDAttr, n.BlobID)
	initAttrs.SetInt64(prefixes.BlobsizeAttr, n.Blobsize)
	initAttrs.SetString(prefixes.StatusPrefix, node.ProcessingStatus+upload.Session.ID)

	// update node metadata with new blobid etc
	err = n.SetXattrsWithContext(ctx, initAttrs, false)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	// add etag to metadata
	upload.Session.MetaData["etag"], _ = node.CalculateEtag(n, mtime)

	// update nodeid for later
	upload.Session.NodeID = n.ID

	if err := upload.Session.Persist(ctx); err != nil {
		return nil, err
	}

	return n, nil
}

func initNewNode(upload *Upload, n *node.Node, fsize uint64) (*lockedfile.File, error) {
	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
		return nil, err
	}

	// create and write lock new node metadata
	f, err := lockedfile.OpenFile(upload.lu.MetadataBackend().LockfilePath(n.InternalPath()), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// we also need to touch the actual node file here it stores the mtime of the resource
	h, err := os.OpenFile(n.InternalPath(), os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return f, err
	}
	h.Close()

	if _, err := node.CheckQuota(upload.Ctx, n.SpaceRoot, false, 0, fsize); err != nil {
		return f, err
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentPath(), n.Name)
	relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
	log := appctx.GetLogger(upload.Ctx).With().Str("childNameLink", childNameLink).Str("relativeNodePath", relativeNodePath).Logger()
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

	// on a new file the sizeDiff is the fileSize
	upload.Session.SizeDiff = int64(fsize)
	return f, nil
}

func updateExistingNode(upload *Upload, n *node.Node, spaceID string, fsize uint64) (*lockedfile.File, error) {
	targetPath := n.InternalPath()

	// write lock existing node before reading any metadata
	f, err := lockedfile.OpenFile(upload.lu.MetadataBackend().LockfilePath(targetPath), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	old, _ := node.ReadNode(upload.Ctx, upload.lu, spaceID, n.ID, false, nil, false)
	if _, err := node.CheckQuota(upload.Ctx, n.SpaceRoot, true, uint64(old.Blobsize), fsize); err != nil {
		return f, err
	}

	oldNodeMtime, err := old.GetMTime(upload.Ctx)
	if err != nil {
		return f, err
	}
	oldNodeEtag, err := node.CalculateEtag(old, oldNodeMtime)
	if err != nil {
		return f, err
	}

	// When the if-match header was set we need to check if the
	// etag still matches before finishing the upload.
	if upload.Session.HeaderIfMatch != "" && upload.Session.HeaderIfMatch != oldNodeEtag {
		return f, errtypes.Aborted("etag mismatch")
	}

	// When the if-none-match header was set we need to check if any of the
	// etags matches before finishing the upload.
	if upload.Session.HeaderIfNoneMatch != "" {
		if upload.Session.HeaderIfNoneMatch == "*" {
			return f, errtypes.Aborted("etag mismatch, resource exists")
		}
		for _, ifNoneMatchTag := range strings.Split(upload.Session.HeaderIfNoneMatch, ",") {
			if ifNoneMatchTag == oldNodeEtag {
				return f, errtypes.Aborted("etag mismatch")
			}
		}
	}

	// When the if-unmodified-since header was set we need to check if the
	// etag still matches before finishing the upload.
	if upload.Session.HeaderIfUnmodifiedSince != "" {
		ifUnmodifiedSince, err := time.Parse(time.RFC3339Nano, upload.Session.HeaderIfUnmodifiedSince)
		if err != nil {
			return f, errtypes.InternalError(fmt.Sprintf("failed to parse if-unmodified-since time: %s", err))
		}

		if oldNodeMtime.After(ifUnmodifiedSince) {
			return f, errtypes.Aborted("if-unmodified-since mismatch")
		}
	}

	upload.Session.SizeDiff = int64(fsize) - old.Blobsize
	upload.Session.VersionsPath = upload.lu.InternalPath(spaceID, n.ID+node.RevisionIDDelimiter+oldNodeMtime.UTC().Format(time.RFC3339Nano))

	// create version node
	if _, err := os.Create(upload.Session.VersionsPath); err != nil {
		return f, err
	}

	// copy blob metadata to version node
	if err := upload.lu.CopyMetadataWithSourceLock(upload.Ctx, targetPath, upload.Session.VersionsPath, func(attributeName string, value []byte) (newValue []byte, copy bool) {
		return value, strings.HasPrefix(attributeName, prefixes.ChecksumPrefix) ||
			attributeName == prefixes.TypeAttr ||
			attributeName == prefixes.BlobIDAttr ||
			attributeName == prefixes.BlobsizeAttr ||
			attributeName == prefixes.MTimeAttr
	}, f, true); err != nil {
		return f, err
	}

	// keep mtime from previous version
	if err := os.Chtimes(upload.Session.VersionsPath, oldNodeMtime, oldNodeMtime); err != nil {
		return f, errtypes.InternalError(fmt.Sprintf("failed to change mtime of version node: %s", err))
	}

	return f, nil
}

// lookupNode looks up nodes by path.
// This method can also handle lookups for paths which contain chunking information.
func lookupNode(ctx context.Context, spaceRoot *node.Node, path string, lu *lookup.Lookup) (*node.Node, error) {
	p := path
	isChunked := chunking.IsChunked(path)
	if isChunked {
		chunkInfo, err := chunking.GetChunkBLOBInfo(path)
		if err != nil {
			return nil, err
		}
		p = chunkInfo.Path
	}

	n, err := lu.WalkPath(ctx, spaceRoot, p, true, func(ctx context.Context, n *node.Node) error { return nil })
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error walking path")
	}

	if isChunked {
		n.Name = filepath.Base(path)
	}
	return n, nil
}

// Progress adapts the persisted upload metadata for the UploadSessionLister interface
type Progress struct {
	Path       string
	Session    tus.Session
	Processing bool
}

// ID implements the storage.UploadSession interface
func (p Progress) ID() string {
	return p.Session.ID
}

// Filename implements the storage.UploadSession interface
func (p Progress) Filename() string {
	return p.Session.MetaData["filename"]
}

// Size implements the storage.UploadSession interface
func (p Progress) Size() int64 {
	return p.Session.Size
}

// Offset implements the storage.UploadSession interface
func (p Progress) Offset() int64 {
	return p.Session.Offset
}

// Reference implements the storage.UploadSession interface
func (p Progress) Reference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: p.Session.ProviderID,
			SpaceId:   p.Session.SpaceRoot,
			OpaqueId:  p.Session.NodeID,
		},
	}
}

// Executant implements the storage.UploadSession interface
func (p Progress) Executant() userpb.UserId {
	return userpb.UserId{
		Idp:      p.Session.ExecutantIdp,
		OpaqueId: p.Session.ExecutantID,
		Type:     utils.UserTypeMap(p.Session.ExecutantType),
	}
}

// SpaceOwner implements the storage.UploadSession interface
func (p Progress) SpaceOwner() *userpb.UserId {
	return &userpb.UserId{
		// idp and type do not seem to be consumed and the node currently only stores the user id anyway
		OpaqueId: p.Session.SpaceOwnerOrManager,
	}
}

// Expires implements the storage.UploadSession interface
func (p Progress) Expires() time.Time {
	return p.Session.Expires
}

// IsProcessing implements the storage.UploadSession interface
func (p Progress) IsProcessing() bool {
	return p.Processing
}

// Purge implements the storage.UploadSession interface
func (p Progress) Purge(ctx context.Context) error {
	binPath := filepath.Join(filepath.Dir(p.Path), p.Session.ID) // FIXME
	berr := os.Remove(binPath)
	if berr != nil {
		appctx.GetLogger(ctx).Error().Str("id", p.Session.ID).Interface("path", binPath).Msg("Decomposedfs: could not purge bin path for upload session")
	}

	// remove upload metadata
	merr := p.Session.Purge(ctx)
	if merr != nil {
		appctx.GetLogger(ctx).Error().Str("id", p.Session.ID).Interface("path", p.Path).Msg("Decomposedfs: could not purge metadata path for upload session")
	}

	return stderrors.Join(berr, merr)
}
