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
	"github.com/pkg/xattr"
)

// SetXattrs sets multiple extended attributes on the write-through cache/node
func (n *Node) SetXattrs(attribs map[string]string, acquireLock bool) (err error) {
	if n.xattrsCache != nil {
		for k, v := range attribs {
			n.xattrsCache[k] = v
		}
	}

	return n.lu.MetadataBackend().SetMultiple(n.InternalPath(), attribs, acquireLock)
}

// SetXattr sets an extended attribute on the write-through cache/node
func (n *Node) SetXattr(key, val string) (err error) {
	if n.xattrsCache != nil {
		n.xattrsCache[key] = val
	}

	return n.lu.MetadataBackend().Set(n.InternalPath(), key, val)
}

// RemoveXattr removes an extended attribute from the write-through cache/node
func (n *Node) RemoveXattr(key string) error {
	if n.xattrsCache != nil {
		delete(n.xattrsCache, key)
	}
	return n.lu.MetadataBackend().Remove(n.InternalPath(), key)
}

// Xattrs returns the extended attributes of the node. If the attributes have already
// been cached they are not read from disk again.
func (n *Node) Xattrs() (map[string]string, error) {
	if n.xattrsCache != nil {
		return n.xattrsCache, nil
	}

	attrs, err := n.lu.MetadataBackend().All(n.InternalPath())
	if err != nil {
		return nil, err
	}
	n.xattrsCache = attrs
	return n.xattrsCache, nil
}

// Xattr returns an extended attribute of the node. If the attributes have already
// been cached it is not read from disk again.
func (n *Node) Xattr(key string) (string, error) {
	if n.xattrsCache == nil {
		attrs, err := n.lu.MetadataBackend().All(n.InternalPath())
		if err != nil {
			return "", err
		}
		n.xattrsCache = attrs
	}

	if val, ok := n.xattrsCache[key]; ok {
		return val, nil
	}
	// wrap the error as xattr does
	return "", &xattr.Error{Op: "xattr.get", Path: n.InternalPath(), Name: key, Err: xattr.ENOATTR}
}
