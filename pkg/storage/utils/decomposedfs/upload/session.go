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

	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/tus"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/lockedfile"
)

// UpdateMetadata will create the target node for the Upload
// - if the node does not exist it is created and assigned an id, no blob id?
// - then always write out a revision node
// - when postprocessing finishes copy metadata to node and replace latest revision node with previous blob info. if blobid is empty delete previous revision completely?
func UpdateMetadata(ctx context.Context, lu *lookup.Lookup, uploadID string, size int64, uploadSession tus.Session) (tus.Session, *node.Node, error) {
	ctx, span := tracer.Start(ctx, "UpdateMetadata")
	defer span.End()
	log := appctx.GetLogger(ctx).With().Str("uploadID", uploadID).Logger()

	// check lock
	if uploadSession.LockID != "" {
		ctx = ctxpkg.ContextSetLockID(ctx, uploadSession.LockID)
	}

	var err error
	var n *node.Node
	var nodeHandle *lockedfile.File
	if uploadSession.NodeID == "" {
		// we need to check if the node exists via parentid & child name
		p, err := node.ReadNode(ctx, lu, uploadSession.SpaceRoot, uploadSession.NodeParentID, false, nil, true)
		if err != nil {
			log.Error().Err(err).Msg("could not read parent node")
			return tus.Session{}, nil, err
		}
		if !p.Exists {
			return tus.Session{}, nil, errtypes.PreconditionFailed("parent does not exist")
		}
		n, err = p.Child(ctx, uploadSession.Filename)
		if err != nil {
			log.Error().Err(err).Msg("could not read child node")
			return tus.Session{}, nil, err
		}
		if !n.Exists {
			n.ID = uuid.New().String()
			nodeHandle, err = initNewNode(ctx, lu, uploadID, uploadSession.MTime, n)
			if err != nil {
				log.Error().Err(err).Msg("could not init new node")
				return tus.Session{}, nil, err
			}
		} else {
			nodeHandle, err = openExistingNode(ctx, lu, n)
			if err != nil {
				log.Error().Err(err).Msg("could not open existing node")
				return tus.Session{}, nil, err
			}
		}
	}

	if nodeHandle == nil {
		n, err = node.ReadNode(ctx, lu, uploadSession.SpaceRoot, uploadSession.NodeID, false, nil, true)
		if err != nil {
			log.Error().Err(err).Msg("could not read parent node")
			return tus.Session{}, nil, err
		}
		nodeHandle, err = openExistingNode(ctx, lu, n)
		if err != nil {
			log.Error().Err(err).Msg("could not open existing node")
			return tus.Session{}, nil, err
		}
	}
	defer func() {
		if nodeHandle == nil {
			return
		}
		if err := nodeHandle.Close(); err != nil {
			log.Error().Err(err).Str("nodeid", n.ID).Str("parentid", n.ParentID).Msg("could not close lock")
		}
	}()

	err = validateRequest(ctx, size, uploadSession, n)
	if err != nil {
		return tus.Session{}, nil, err
	}

	// set processing status of node
	nodeAttrs := node.Attributes{}
	// store Blobsize in node so we can propagate a sizediff
	// do not yet update the blobid ... urgh this is fishy
	nodeAttrs.SetString(prefixes.StatusPrefix, node.ProcessingStatus+uploadID)
	err = n.SetXattrsWithContext(ctx, nodeAttrs, false)
	if err != nil {
		return tus.Session{}, nil, errors.Wrap(err, "Decomposedfs: could not write metadata")
	}

	uploadSession.BlobSize = size
	// TODO we should persist all versions as writes with ranges and the blobid in the node metadata

	err = uploadSession.Persist(ctx)
	if err != nil {
		return tus.Session{}, nil, errors.Wrap(err, "Decomposedfs: could not write upload metadata")
	}

	return uploadSession, n, nil
}
