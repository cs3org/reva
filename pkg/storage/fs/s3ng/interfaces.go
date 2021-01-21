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

package s3ng

import (
	"context"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
)

// TODO the different aspects of a storage: Tree, Lookup and Permissions should be able to be reusable
// Below is a start of Interfaces that needs to be worked out further

// TreePersistence is used to manage a tree hierarchy
type TreePersistence interface {
	GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	GetMD(ctx context.Context, node *node.Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *node.Node) ([]*node.Node, error)
	//CreateHome(owner *userpb.UserId) (n *node.Node, err error)
	CreateDir(ctx context.Context, node *node.Node) (err error)
	//CreateReference(ctx context.Context, node *node.Node, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error)
	Delete(ctx context.Context, node *node.Node) (err error)

	Propagate(ctx context.Context, node *node.Node) (err error)
}

// Lookup is used to encapsulate path transformations
/*
type Lookup interface {
	NodeFromResource(ctx context.Context, ref *provider.Reference) (node *node.Node, err error)
	NodeFromID(ctx context.Context, id *provider.ResourceId) (node *node.Node, err error)
	NodeFromPath(ctx context.Context, fn string) (node *node.Node, err error)
	Path(ctx context.Context, node *node.Node) (path string, err error)

	// HomeNode returns the currently logged in users home node
	// requires EnableHome to be true
	HomeNode(ctx context.Context) (node *node.Node, err error)

	// RootNode returns the storage root node
	RootNode(ctx context.Context) (node *node.Node, err error)

	// HomeOrRootNode returns the users home node when home support is enabled.
	// it returns the storages root node otherwise
	HomeOrRootNode(ctx context.Context) (node *node.Node, err error)
}
*/
