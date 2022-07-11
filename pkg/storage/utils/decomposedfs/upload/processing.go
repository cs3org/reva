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
	"encoding/json"
	iofs "io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	tusd "github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
	HasPermission(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) (can bool, err error)
}

// New returns a new processing instance
func New(ctx context.Context, info tusd.FileInfo, lu *lookup.Lookup, tp Tree, p PermissionsChecker, fsRoot string, o options.PostprocessingOptions, pub events.Publisher) (upload *Upload, err error) {

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("info", info).Msg("Decomposedfs: NewUpload")

	if info.MetaData["filename"] == "" {
		return nil, errors.New("Decomposedfs: missing filename in metadata")
	}
	if info.MetaData["dir"] == "" {
		return nil, errors.New("Decomposedfs: missing dir in metadata")
	}

	n, err := lu.NodeFromSpaceID(ctx, &provider.ResourceId{
		SpaceId:  info.Storage["SpaceRoot"],
		OpaqueId: info.Storage["SpaceRoot"],
	})
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error getting space root node")
	}

	n, err = lookupNode(ctx, n, filepath.Join(info.MetaData["dir"], info.MetaData["filename"]), lu)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error walking path")
	}

	log.Debug().Interface("info", info).Interface("node", n).Msg("Decomposedfs: resolved filename")

	// the parent owner will become the new owner
	parent, perr := n.Parent()
	if perr != nil {
		return nil, errors.Wrap(perr, "Decomposedfs: error getting parent "+n.ParentID)
	}

	// check permissions
	var ok bool
	if n.Exists {
		// check permissions of file to be overwritten
		ok, err = p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
			return rp.InitiateFileUpload
		})
	} else {
		// check permissions of parent
		ok, err = p.HasPermission(ctx, parent, func(rp *provider.ResourcePermissions) bool {
			return rp.InitiateFileUpload
		})
	}
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	// are we trying to overwriting a folder with a file?
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

	binPath := filepath.Join(fsRoot, "uploads", info.ID)
	usr := ctxpkg.ContextMustGetUser(ctx)

	var spaceRoot string
	if info.Storage != nil {
		if spaceRoot, ok = info.Storage["SpaceRoot"]; !ok {
			spaceRoot = n.SpaceRoot.ID
		}
	} else {
		spaceRoot = n.SpaceRoot.ID
	}

	info.Storage = map[string]string{
		"Type":    "OCISStore",
		"BinPath": binPath,

		"NodeId":       n.ID,
		"NodeParentId": n.ParentID,
		"NodeName":     n.Name,
		"SpaceRoot":    spaceRoot,

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

	u := buildUpload(ctx, info, binPath, filepath.Join(fsRoot, "uploads", info.ID+".info"), lu, tp, pub, o.AsyncFileUploads)

	// writeInfo creates the file by itself if necessary
	err = u.writeInfo()
	if err != nil {
		return nil, err
	}

	return u, nil
}

// Get returns the Upload for the given upload id
func Get(ctx context.Context, id string, lu *lookup.Lookup, tp Tree, fsRoot string, o options.PostprocessingOptions, pub events.Publisher) (*Upload, error) {
	infoPath := filepath.Join(fsRoot, "uploads", id+".info")

	info := tusd.FileInfo{}
	data, err := ioutil.ReadFile(infoPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = tusd.ErrNotFound
		}
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
			Type:     utils.UserTypeMap(info.Storage["UserType"]),
		},
		Username: info.Storage["UserName"],
	}

	ctx = ctxpkg.ContextSetUser(ctx, u)
	// TODO configure the logger the same way ... store and add traceid in file info

	var opts []logger.Option
	opts = append(opts, logger.WithLevel(info.Storage["LogLevel"]))
	opts = append(opts, logger.WithWriter(os.Stderr, logger.ConsoleMode))
	l := logger.New(opts...)

	sub := l.With().Int("pid", os.Getpid()).Logger()

	ctx = appctx.WithLogger(ctx, &sub)

	up := buildUpload(ctx, info, info.Storage["BinPath"], infoPath, lu, tp, pub, o.AsyncFileUploads)
	return up, nil
}

// CreateNodeForUpload will create the target node for the Upload
func CreateNodeForUpload(upload *Upload) (*node.Node, error) {
	fi, err := os.Stat(upload.binPath)
	if err != nil {
		return nil, err
	}

	fsize := fi.Size()
	spaceID := upload.Info.Storage["SpaceRoot"]
	n := node.New(
		spaceID,
		upload.Info.Storage["NodeId"],
		upload.Info.Storage["NodeParentId"],
		upload.Info.Storage["NodeName"],
		fsize,
		"",
		nil,
		upload.lu,
	)
	n.SpaceRoot = node.New(spaceID, spaceID, "", "", 0, "", nil, upload.lu)

	// check lock
	if err := n.CheckLock(upload.Ctx); err != nil {
		return nil, err
	}

	switch n.ID {
	case "":
		n.ID = uuid.New().String()

		// create folder structure (if needed)
		if err := os.MkdirAll(filepath.Dir(n.InternalPath()), 0700); err != nil {
			return nil, err
		}

		if _, err := os.Create(n.InternalPath()); err != nil {
			return nil, err
		}

		if _, err = node.CheckQuota(n.SpaceRoot, false, 0, uint64(fsize)); err != nil {
			return nil, err
		}

		// link child name to parent if it is new
		childNameLink := filepath.Join(n.ParentInternalPath(), n.Name)
		var link string
		link, err = os.Readlink(childNameLink)
		if err == nil && link != "../"+n.ID {
			if err = os.Remove(childNameLink); err != nil {
				return nil, errors.Wrap(err, "Decomposedfs: could not remove symlink child entry")
			}
		}
		if errors.Is(err, iofs.ErrNotExist) || link != "../"+n.ID {
			relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
			if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
				return nil, errors.Wrap(err, "Decomposedfs: could not symlink child entry")
			}
		}
		err = n.WriteAllNodeMetadata()
		if err != nil {
			return nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
		}

	default:
		old, _ := node.ReadNode(upload.Ctx, upload.lu, spaceID, n.ID, false)
		oldsize := uint64(old.Blobsize)
		upload.oldsize = &oldsize

		if _, err = node.CheckQuota(n.SpaceRoot, true, oldsize, uint64(fsize)); err != nil {
			return nil, err
		}
	}

	return n, upload.tp.Propagate(upload.Ctx, n)
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
