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
	"os"
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
)

// Path implements transformations from filepath to node and back
type Path struct {
	// ocis fs works on top of a dir of uuid nodes
	Root string `mapstructure:"root"`

	// UserLayout describles the relative path from the storages root node to the users home node.
	UserLayout string `mapstructure:"user_layout"`

	// TODO NodeLayout option to save nodes as eg. nodes/1d/d8/1dd84abf-9466-4e14-bb86-02fc4ea3abcf

	// EnableHome enables the creation of home directories.
	EnableHome  bool   `mapstructure:"enable_home"`
	ShareFolder string `mapstructure:"share_folder"`
}

// NodeFromResource takes in a request path or request id and converts it to a Node
func (pw *Path) NodeFromResource(ctx context.Context, ref *provider.Reference) (*Node, error) {
	if ref.GetPath() != "" {
		return pw.NodeFromPath(ctx, ref.GetPath())
	}

	if ref.GetId() != nil {
		return pw.NodeFromID(ctx, ref.GetId())
	}

	// reference is invalid
	return nil, fmt.Errorf("invalid reference %+v", ref)
}

// NodeFromPath converts a filename into a Node
func (pw *Path) NodeFromPath(ctx context.Context, fn string) (node *Node, err error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("fn", fn).Msg("NodeFromPath()")
	if pw.EnableHome {
		node, err = pw.HomeNode(ctx)
	} else {
		node, err = pw.RootNode(ctx)
	}
	if err != nil {
		return
	}

	if fn != "/" {
		// walk the path
		segments := strings.Split(strings.TrimLeft(fn, "/"), "/")
		for i := range segments {
			if node, err = node.Child(segments[i]); err != nil {
				log.Error().Err(err).Interface("node", node).Str("segment", segments[i]).Msg("NodeFromPath()")
				break
			}
			log.Debug().Interface("node", node).Str("segment", segments[i]).Msg("NodeFromPath()")
		}
	}

	return
}

// NodeFromID returns the internal path for the id
func (pw *Path) NodeFromID(ctx context.Context, id *provider.ResourceId) (n *Node, err error) {
	if id == nil || id.OpaqueId == "" {
		return nil, fmt.Errorf("invalid resource id %+v", id)
	}
	return ReadNode(ctx, pw, id.OpaqueId)
}

// Path returns the path for node
func (pw *Path) Path(ctx context.Context, n *Node) (p string, err error) {
	var root *Node
	if pw.EnableHome {
		root, err = pw.HomeNode(ctx)
	} else {
		root, err = pw.RootNode(ctx)
	}
	if err != nil {
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

// readRootLink reads the symbolic link and extracts the node id
func (pw *Path) readRootLink(root string) (node *Node, err error) {
	// A root symlink looks like `../nodes/76455834-769e-412a-8a01-68f265365b79`
	link, err := os.Readlink(root)
	if os.IsNotExist(err) {
		err = errtypes.NotFound(root)
		return
	}

	// extract the nodeID
	if strings.HasPrefix(link, "../nodes/") {
		node = &Node{
			pw:       pw,
			ID:       filepath.Base(link),
			ParentID: "root",
			Exists:   true,
		}
	} else {
		err = fmt.Errorf("ocisfs: expected '../nodes/ prefix, got' %+v", link)
	}
	return
}

// RootNode returns the root node of a tree
func (pw *Path) RootNode(ctx context.Context) (node *Node, err error) {
	return &Node{
		pw:       pw,
		ID:       "root",
		Name:     "",
		ParentID: "",
		Exists:   true,
	}, nil
}

// HomeNode returns the home node of a user
func (pw *Path) HomeNode(ctx context.Context) (node *Node, err error) {
	if !pw.EnableHome {
		return nil, errtypes.NotSupported("ocisfs: home supported disabled")
	}

	if node, err = pw.RootNode(ctx); err != nil {
		return
	}
	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, pw.UserLayout)

	segments := strings.Split(layout, "/")
	for i := range segments {
		if node, err = node.Child(segments[i]); err != nil {
			return
		}
	}
	return
}
