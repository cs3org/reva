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
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
)

// SetTrashOrigin stores the original path to use for restoring
func (n *Node) SetTrashOrigin(origin string) error {
	return n.SetXattr(prefixes.TrashOriginAttr, origin)
}

// GetTrashOrigin returns the original path to use for restoring
func (n *Node) GetTrashOrigin() (string, error) {
	return n.Xattr(prefixes.TrashOriginAttr)
}

// RemoveTrashOrigin unsets the original path to use for restoring
func (n *Node) RemoveTrashOrigin() error {
	return n.RemoveXattr(prefixes.TrashOriginAttr)
	/* TODO ignore unset error?
	if xattrs.IsAttrUnset(err) {
		return nil // already gone, ignore
	}
	return err
	*/
}
