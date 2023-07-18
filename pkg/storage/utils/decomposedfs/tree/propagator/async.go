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

package propagator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/google/renameio"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/lockedfile"
	"github.com/rs/zerolog"
	"github.com/shamaton/msgpack/v2"
)

type AsyncPropagator struct {
	treeSizeAccounting bool
	treeTimeAccounting bool
	lookup             lookup.PathLookup
}

type Change struct {
	SyncTime time.Time
	SizeDiff int64
}

func NewAsyncPropagator(treeSizeAccounting, treeTimeAccounting bool, lookup lookup.PathLookup) AsyncPropagator {
	return AsyncPropagator{
		treeSizeAccounting: treeSizeAccounting,
		treeTimeAccounting: treeTimeAccounting,
		lookup:             lookup,
	}
}

func (p AsyncPropagator) Propagate(ctx context.Context, n *node.Node, sizeDiff int64) error {
	ctx, span := tracer.Start(ctx, "Propagate")
	defer span.End()
	log := appctx.GetLogger(ctx).With().
		Str("method", "async.Propagate").
		Str("spaceid", n.SpaceID).
		Str("nodeid", n.ID).
		Int64("sizeDiff", sizeDiff).
		Logger()

	if !p.treeTimeAccounting && (!p.treeSizeAccounting || sizeDiff == 0) {
		// no propagation enabled
		log.Debug().Msg("propagation disabled or nothing to propagate")
		return nil
	}

	// add a change to the parent node
	c := Change{
		// use a sync time and don't rely on the mtime of the current node, as the stat might not change when a rename happened too quickly
		SyncTime: time.Now().UTC(),
		SizeDiff: sizeDiff,
	}
	go p.propagateChange(ctx, n.SpaceID, n.ParentID, c, log)

	return nil
}

func (p AsyncPropagator) propagateChange(ctx context.Context, spaceID, nodeID string, change Change, log zerolog.Logger) {
	// add a change to the parent node
	changePath := p.changesPath(spaceID, nodeID, uuid.New().String()+".mpk")

	data, err := msgpack.Marshal(change)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal Change")
		return
	}

	_, subspan := tracer.Start(ctx, "write changes file")
	ready := false
	_ = os.MkdirAll(filepath.Dir(changePath), 0700)
	for retries := 0; retries <= 5; retries += 1 {
		err := renameio.WriteFile(changePath, data, 0644)
		if err == nil {
			ready = true
			break
		}
		_ = os.MkdirAll(filepath.Dir(changePath), 0700)
	}

	if !ready {
		log.Error().Err(err).Msg("failed to write Change to disk")
		return
	}
	subspan.End()

	p.propagate(ctx, spaceID, nodeID, log)
}

func (p AsyncPropagator) propagate(ctx context.Context, spaceID, nodeID string, log zerolog.Logger) {

	time.Sleep(time.Millisecond * 300) // wait a moment before propagating

	log.Debug().Msg("propagating")

	// add a change to the parent node
	changeDirPath := p.changesPath(spaceID, nodeID, "")

	// TODO what if a rename is already in progress? we need to create a .processing.lock file so other goroutines wait for this?

	// first rename the existing node dir
	err := os.Rename(changeDirPath, changeDirPath+".processing")
	if err != nil {
		return // has already been propagated by another goroutine
	}

	d, err := os.Open(changeDirPath + ".processing")
	if err != nil {
		log.Error().Err(err).Msg("Could not open change .processing dir")
		return
	}
	defer d.Close()
	names, err := d.Readdirnames(0)
	if err != nil {
		log.Error().Err(err).Msg("Could not read dirnames")
		return
	}

	pc := Change{}
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(changeDirPath+".processing", name))
		if err != nil {
			log.Error().Err(err).Msg("Could not read change") // TODO what if the file was not yet fully written? retry empty files?
			return
		}
		c := Change{}
		err = msgpack.Unmarshal(b, &c)
		if err != nil {
			log.Error().Err(err).Msg("Could not unmarshal change")
			return
		}
		if c.SyncTime.After(pc.SyncTime) {
			pc.SyncTime = c.SyncTime
		}
		pc.SizeDiff += c.SizeDiff
	}

	// TODO do we need to write an aggregated parentchange file?

	attrs := node.Attributes{}

	var f *lockedfile.File
	// lock parent before reading treesize or tree time

	nodePath := p.changesPath(spaceID, nodeID, "")

	_, subspan := tracer.Start(ctx, "lockedfile.OpenFile")
	lockFilepath := p.lookup.MetadataBackend().LockfilePath(nodePath)
	f, err = lockedfile.OpenFile(lockFilepath, os.O_RDWR|os.O_CREATE, 0600)
	subspan.End()
	if err != nil {
		log.Error().Err(err).
			Str("lock filepath", lockFilepath).
			Msg("Propagation failed. Could not open metadata for node with lock.")
		return
	}
	// always log error if closing node fails
	defer func() {
		// ignore already closed error
		cerr := f.Close()
		if err == nil && cerr != nil && !errors.Is(cerr, os.ErrClosed) {
			err = cerr // only overwrite err with en error from close if the former was nil
		}
	}()

	var n *node.Node
	if n, err = node.ReadNode(ctx, p.lookup, spaceID, nodeID, false, nil, false); err != nil {
		log.Error().Err(err).
			Msg("Propagation failed. Could not read node.")
		return
	}

	// TODO none, sync and async?
	if !n.HasPropagation(ctx) {
		log.Debug().Str("attr", prefixes.PropagationAttr).Msg("propagation attribute not set or unreadable, not propagating")
		// if the attribute is not set treat it as false / none / no propagation
		return
	}

	if p.treeTimeAccounting {
		// update the parent tree time if it is older than the nodes mtime
		updateSyncTime := false

		var tmTime time.Time
		tmTime, err = n.GetTMTime(ctx)
		switch {
		case err != nil:
			// missing attribute, or invalid format, overwrite
			log.Debug().Err(err).
				Msg("could not read tmtime attribute, overwriting")
			updateSyncTime = true
		case tmTime.Before(pc.SyncTime):
			log.Debug().
				Time("tmtime", tmTime).
				Time("stime", pc.SyncTime).
				Msg("parent tmtime is older than node mtime, updating")
			updateSyncTime = true
		default:
			log.Debug().
				Time("tmtime", tmTime).
				Time("stime", pc.SyncTime).
				Dur("delta", pc.SyncTime.Sub(tmTime)).
				Msg("node tmtime is younger than stime, not updating")
		}

		if updateSyncTime {
			// update the tree time of the parent node
			attrs.SetString(prefixes.TreeMTimeAttr, pc.SyncTime.UTC().Format(time.RFC3339Nano))
		}

		attrs.SetString(prefixes.TmpEtagAttr, "")
	}

	// size accounting
	if p.treeSizeAccounting && pc.SizeDiff != 0 {
		var newSize uint64

		// read treesize
		treeSize, err := n.GetTreeSize(ctx)
		switch {
		case metadata.IsAttrUnset(err):
			// fallback to calculating the treesize
			log.Warn().Msg("treesize attribute unset, falling back to calculating the treesize")
			newSize, err = calculateTreeSize(ctx, p.lookup, n.InternalPath())
			if err != nil {
				log.Error().Err(err).
					Msg("Error when calculating treesize of node.") // FIXME wat?
				return
			}
		case err != nil:
			log.Error().Err(err).
				Msg("Failed to propagate treesize change. Error when reading the treesize attribute from node")
			return
		case pc.SizeDiff > 0:
			newSize = treeSize + uint64(pc.SizeDiff)
		case uint64(-pc.SizeDiff) > treeSize:
			// The sizeDiff is larger than the current treesize. Which would result in
			// a negative new treesize. Something must have gone wrong with the accounting.
			// Reset the current treesize to 0.
			log.Error().Uint64("treeSize", treeSize).Int64("sizeDiff", pc.SizeDiff).
				Msg("Error when updating treesize of node. Updated treesize < 0. Reestting to 0")
			newSize = 0
		default:
			newSize = treeSize - uint64(-pc.SizeDiff)
		}

		// update the tree size of the node
		attrs.SetString(prefixes.TreesizeAttr, strconv.FormatUint(newSize, 10))
		log.Debug().Uint64("newSize", newSize).Msg("updated treesize of node")
	}

	if err = n.SetXattrsWithContext(ctx, attrs, false); err != nil {
		log.Error().Err(err).Msg("Failed to update extend attributes of node")
		return
	}

	// Release node lock early, ignore already closed error
	_, subspan = tracer.Start(ctx, "f.Close")
	cerr := f.Close()
	subspan.End()
	if cerr != nil && !errors.Is(cerr, os.ErrClosed) {
		log.Error().Err(cerr).Msg("Failed to close node and release lock")
	}

	err = os.RemoveAll(changeDirPath + ".processing")
	if err != nil {
		log.Error().Err(err).Msg("Could not remove .processing dir")
	}

	if !n.IsSpaceRoot(ctx) { // This does not seem robust as it checks the space name property
		p.propagateChange(ctx, n.SpaceID, n.ParentID, pc, log)
	}
}

func (p AsyncPropagator) changesPath(spaceID, nodeID, filename string) string {
	return filepath.Join(p.lookup.InternalRoot(), "spaces", lookup.Pathify(spaceID, 1, 2), "changes", nodeID, filename)
}
