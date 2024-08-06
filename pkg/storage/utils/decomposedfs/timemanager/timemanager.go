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

package timemanager

import (
	"context"
	"os"
	"time"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
)

type Manager struct {
}

func (dtm *Manager) MTime(ctx context.Context, n *node.Node) (time.Time, error) {
	b, err := n.XattrString(ctx, prefixes.MTimeAttr)
	if err != nil {
		fi, err := os.Lstat(n.InternalPath())
		if err != nil {
			return time.Time{}, err
		}
		return fi.ModTime(), nil
	}
	return time.Parse(time.RFC3339Nano, b)
}

func (dtm *Manager) SetMTime(ctx context.Context, n *node.Node, mtime *time.Time) error {
	if mtime == nil {
		return n.RemoveXattr(ctx, prefixes.MTimeAttr, true)
	}
	return n.SetXattrString(ctx, prefixes.MTimeAttr, mtime.UTC().Format(time.RFC3339Nano))
}

func (dtm *Manager) TMTime(ctx context.Context, n *node.Node) (time.Time, error) {
	b, err := n.XattrString(ctx, prefixes.TreeMTimeAttr)
	if err == nil {
		return time.Parse(time.RFC3339Nano, b)
	}

	// no tmtime, use mtime
	return dtm.MTime(ctx, n)
}

func (dtm *Manager) SetTMTime(ctx context.Context, n *node.Node, tmtime *time.Time) error {
	if tmtime == nil {
		return n.RemoveXattr(ctx, prefixes.TreeMTimeAttr, true)
	}
	return n.SetXattrString(ctx, prefixes.TreeMTimeAttr, tmtime.UTC().Format(time.RFC3339Nano))
}

func (dtm *Manager) CTime(ctx context.Context, n *node.Node) (time.Time, error) {
	// decomposedfs does not differentiate between ctime and mtime
	return dtm.MTime(ctx, n)
}

func (dtm *Manager) SetCTime(ctx context.Context, n *node.Node, mtime *time.Time) error {
	// decomposedfs does not differentiate between ctime and mtime
	return dtm.SetMTime(ctx, n, mtime)
}

func (dtm *Manager) TCTime(ctx context.Context, n *node.Node) (time.Time, error) {
	// decomposedfs does not differentiate between ctime and mtime
	return dtm.TMTime(ctx, n)
}

func (dtm *Manager) SetTCTime(ctx context.Context, n *node.Node, tmtime *time.Time) error {
	// decomposedfs does not differentiate between ctime and mtime
	return dtm.SetTMTime(ctx, n, tmtime)
}

// GetDTime reads the dtime from the extended attributes
func (dtm *Manager) DTime(ctx context.Context, n *node.Node) (tmTime time.Time, err error) {
	b, err := n.XattrString(ctx, prefixes.DTimeAttr)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, b)
}

// SetDTime writes the UTC dtime to the extended attributes or removes the attribute if nil is passed
func (dtm *Manager) SetDTime(ctx context.Context, n *node.Node, t *time.Time) (err error) {
	if t == nil {
		return n.RemoveXattr(ctx, prefixes.DTimeAttr, true)
	}
	return n.SetXattrString(ctx, prefixes.DTimeAttr, t.UTC().Format(time.RFC3339Nano))
}
