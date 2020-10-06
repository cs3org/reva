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
	"fmt"
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
)

// Lookup implements transformations from filepath to node and back
type Lookup struct {
	Options *Options
}

// NodeFromResource takes in a request path or request id and converts it to a Node
func (lu *Lookup) NodeFromResource(ctx context.Context, ref *provider.Reference) (*Node, error) {
	if ref.GetPath() != "" {
		return lu.NodeFromPath(ctx, ref.GetPath())
	}

	if ref.GetId() != nil {
		return lu.NodeFromID(ctx, ref.GetId())
	}

	// reference is invalid
	return nil, fmt.Errorf("invalid reference %+v", ref)
}

// NodeFromPath converts a filename into a Node
func (lu *Lookup) NodeFromPath(ctx context.Context, fn string) (node *Node, err error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("fn", fn).Msg("NodeFromPath()")

	if node, err = lu.HomeOrRootNode(ctx); err != nil {
		return
	}

	if fn != "/" {
		node, err = lu.WalkPath(ctx, node, fn, func(ctx context.Context, n *Node) error {
			log.Debug().Interface("node", n).Msg("NodeFromPath() walk")
			return nil
		})
	}

	return
}

// NodeFromID returns the internal path for the id
func (lu *Lookup) NodeFromID(ctx context.Context, id *provider.ResourceId) (n *Node, err error) {
	if id == nil || id.OpaqueId == "" {
		return nil, fmt.Errorf("invalid resource id %+v", id)
	}
	return ReadNode(ctx, lu, id.OpaqueId)
}

// Path returns the path for node
func (lu *Lookup) Path(ctx context.Context, n *Node) (p string, err error) {
	var root *Node
	if root, err = lu.HomeOrRootNode(ctx); err != nil {
		return
	}
	for n.ID != root.ID {
		p = filepath.Join(n.Name, p)
		if n, err = n.Parent(); err != nil {
			appctx.GetLogger(ctx).
				Error().Err(err).
				Str("path", p).
				Interface("node", n).
				Msg("Path()")
			return
		}
	}
	return
}

// RootNode returns the root node of the storage
func (lu *Lookup) RootNode(ctx context.Context) (node *Node, err error) {
	return &Node{
		lu:       lu,
		ID:       "root",
		Name:     "",
		ParentID: "",
		Exists:   true,
	}, nil
}

// HomeNode returns the home node of a user
func (lu *Lookup) HomeNode(ctx context.Context) (node *Node, err error) {
	if !lu.Options.EnableHome {
		return nil, errtypes.NotSupported("ocisfs: home supported disabled")
	}

	if node, err = lu.RootNode(ctx); err != nil {
		return
	}
	node, err = lu.WalkPath(ctx, node, lu.mustGetUserLayout(ctx), nil)
	return
}

// WalkPath calls n.Child(segment) on every path segment in p starting at the node r
// If a function f is given it will be executed for every segment node, but not the root node r
func (lu *Lookup) WalkPath(ctx context.Context, r *Node, p string, f func(ctx context.Context, n *Node) error) (*Node, error) {
	segments := strings.Split(strings.Trim(p, "/"), "/")
	var err error
	for i := range segments {
		if r, err = r.Child(segments[i]); err != nil {
			return r, err
		}
		// if an intermediate node is missing return not found
		if !r.Exists && i < len(segments)-1 {
			return r, errtypes.NotFound(segments[i])
		}
		if f != nil {
			if err = f(ctx, r); err != nil {
				return r, err
			}
		}
	}
	return r, nil
}

// HomeOrRootNode returns the users home node when home support is enabled.
// it returns the storages root node otherwise
func (lu *Lookup) HomeOrRootNode(ctx context.Context) (node *Node, err error) {
	if lu.Options.EnableHome {
		return lu.HomeNode(ctx)
	}
	return lu.RootNode(ctx)
}

func (lu *Lookup) mustGetUserLayout(ctx context.Context) string {
	u := user.ContextMustGetUser(ctx)
	return templates.WithUser(u, lu.Options.UserLayout)
}

func (lu *Lookup) toInternalPath(id string) string {
	return filepath.Join(lu.Options.Root, "nodes", id)
}
