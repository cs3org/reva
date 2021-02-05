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

package helpers

import (
	"context"
	"os"
	"path"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/tree"
	ruser "github.com/cs3org/reva/pkg/user"
)

// CreateEmptyNodeForOtherUser creates a home and an empty node for a new user
func CreateEmptyNodeForOtherUser(id, name string, fs storage.FS, lookup tree.PathLookup) (*node.Node, error) {
	user := &userpb.User{
		Id: &userpb.UserId{
			Idp:      "idp",
			OpaqueId: "userid2",
		},
		Username: "otheruser",
	}
	ctx := ruser.ContextSetUser(context.Background(), user)
	err := fs.CreateHome(ctx)
	if err != nil {
		return nil, err
	}
	return CreateEmptyNode(ctx, id, name, user.Id, lookup)
}

// CreateEmptyNode creates a home and an empty node for the given context
func CreateEmptyNode(ctx context.Context, id, name string, userid *userpb.UserId, lookup tree.PathLookup) (*node.Node, error) {
	root, err := lookup.HomeOrRootNode(ctx)
	if err != nil {
		return nil, err
	}

	n := node.New(id, root.ID, name, 1234, userid, lookup)
	p, err := n.Parent()
	if err != nil {
		return nil, err
	}

	// Create an empty file node
	_, err = os.OpenFile(n.InternalPath(), os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	// ... and an according link in the parent
	err = os.Symlink("../"+n.ID, path.Join(p.InternalPath(), n.Name))
	if err != nil {
		return nil, err
	}

	err = n.WriteMetadata(userid)
	if err != nil {
		return nil, err
	}

	return n, nil
}
