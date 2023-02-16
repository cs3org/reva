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

package node

import (
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
	"github.com/cs3org/reva/v2/pkg/utils"
)

// SetArbitraryMetadata populates a given key with its value.
func (n *Node) SetArbitraryMetadata(key string, val string) error {
	return n.SetXattr(prefixes.MetadataPrefix+key, val)
}

// UnsetArbitraryMetadata removes a given key.
func (n *Node) UnsetArbitraryMetadata(key string) error {
	err := n.RemoveXattr(prefixes.MetadataPrefix + key)
	if xattrs.IsAttrUnset(err) {
		return nil // already gone, ignore
	}
	return err
}

// RemoveFavorite removes the favorite flag for the given user
func (n *Node) RemoveFavorite(uid *userpb.UserId) error {
	fa := fmt.Sprintf("%s:%s:%s@%s", prefixes.FavPrefix, utils.UserTypeToString(uid.GetType()), uid.GetOpaqueId(), uid.GetIdp())
	err := n.RemoveXattr(fa)
	if xattrs.IsAttrUnset(err) {
		return nil // already gone, ignore
	}
	return err
}
