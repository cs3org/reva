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
	"strconv"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// GetTreeSize reads the treesize from the extended attributes
func (n *Node) GetTreeSize() (treesize uint64, err error) {
	var b string
	if b, err = n.Xattr(prefixes.TreesizeAttr); err != nil {
		return
	}
	return strconv.ParseUint(b, 10, 64)
}

// SetTreeSize writes the treesize to the extended attributes
func (n *Node) SetTreeSize(ts uint64) (err error) {
	return n.SetXattr(prefixes.TreesizeAttr, strconv.FormatUint(ts, 10))
}

// GetBlobSize reads the blobsize from the extended attributes
func (n *Node) GetBlobSize() (blebsize uint64, err error) {
	var b string
	if b, err = n.Xattr(prefixes.BlobsizeAttr); err != nil {
		return
	}
	return strconv.ParseUint(b, 10, 64)
}

// RevisionBlobSize reads the blobsize for a specific revision of the node
func (n *Node) RevisionBlobSize(revisionKey string) (int64, error) {
	var val string
	var ok bool
	if n.xattrsCache == nil {
		var err error
		val, err = xattrs.Get(n.InternalPath()+RevisionIDDelimiter+revisionKey, prefixes.BlobsizeAttr)
		if err != nil {
			return 0, err
		}
	} else {
		if val, ok = n.xattrsCache[prefixes.BlobsizeAttr]; !ok {
			// wrap the error as xattr does
			return 0, &xattr.Error{Op: "xattr.get", Path: n.InternalPath(), Name: prefixes.BlobsizeAttr, Err: xattr.ENOATTR}
		}
	}
	blobSize, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid blobsize xattr format")
	}
	return blobSize, nil
}
