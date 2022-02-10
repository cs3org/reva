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

package xattrs

import (
	"os"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"
)

// Declare a list of xattr keys
// TODO the below comment is currently copied from the owncloud driver, revisit
// Currently,extended file attributes have four separated
// namespaces (user, trusted, security and system) followed by a dot.
// A non root user can only manipulate the user. namespace, which is what
// we will use to store ownCloud specific metadata. To prevent name
// collisions with other apps We are going to introduce a sub namespace
// "user.ocis."
const (
	OcisPrefix    string = "user.ocis."
	ParentidAttr  string = OcisPrefix + "parentid"
	OwnerIDAttr   string = OcisPrefix + "owner.id"
	OwnerIDPAttr  string = OcisPrefix + "owner.idp"
	OwnerTypeAttr string = OcisPrefix + "owner.type"
	// the base name of the node
	// updated when the file is renamed or moved
	NameAttr string = OcisPrefix + "name"

	BlobIDAttr   string = OcisPrefix + "blobid"
	BlobsizeAttr string = OcisPrefix + "blobsize"

	// grantPrefix is the prefix for sharing related extended attributes
	GrantPrefix         string = OcisPrefix + "grant."
	GrantUserAcePrefix  string = OcisPrefix + "grant." + UserAcePrefix
	GrantGroupAcePrefix string = OcisPrefix + "grant." + GroupAcePrefix
	MetadataPrefix      string = OcisPrefix + "md."

	// favorite flag, per user
	FavPrefix string = OcisPrefix + "fav."

	// a temporary etag for a folder that is removed when the mtime propagation happens
	TmpEtagAttr     string = OcisPrefix + "tmp.etag"
	ReferenceAttr   string = OcisPrefix + "cs3.ref"      // arbitrary metadata
	ChecksumPrefix  string = OcisPrefix + "cs."          // followed by the algorithm, eg. ocis.cs.sha1
	TrashOriginAttr string = OcisPrefix + "trash.origin" // trash origin

	// we use a single attribute to enable or disable propagation of both: synctime and treesize
	// The propagation attribute is set to '1' at the top of the (sub)tree. Propagation will stop at
	// that node.
	PropagationAttr string = OcisPrefix + "propagation"

	// the tree modification time of the tree below this node,
	// propagated when synctime_accounting is true and
	// user.ocis.propagation=1 is set
	// stored as a readable time.RFC3339Nano
	TreeMTimeAttr string = OcisPrefix + "tmtime"

	// the size of the tree below this node,
	// propagated when treesize_accounting is true and
	// user.ocis.propagation=1 is set
	// stored as uint64, little endian
	TreesizeAttr string = OcisPrefix + "treesize"

	// the quota for the storage space / tree, regardless who accesses it
	QuotaAttr string = OcisPrefix + "quota"

	// the name given to a storage space. It should not contain any semantics as its only purpose is to be read.
	SpaceNameAttr        string = OcisPrefix + "space.name"
	SpaceDescriptionAttr string = OcisPrefix + "space.description"
	SpaceReadmeAttr      string = OcisPrefix + "space.readme"
	SpaceImageAttr       string = OcisPrefix + "space.image"

	UserAcePrefix  string = "u:"
	GroupAcePrefix string = "g:"
)

// ReferenceFromAttr returns a CS3 reference from xattr of a node.
// Supported formats are: "cs3:storageid/nodeid"
func ReferenceFromAttr(b []byte) (*provider.Reference, error) {
	return refFromCS3(b)
}

// refFromCS3 creates a CS3 reference from a set of bytes. This method should remain private
// and only be called after validation because it can potentially panic.
func refFromCS3(b []byte) (*provider.Reference, error) {
	parts := string(b[4:])
	return &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: strings.Split(parts, "/")[0],
			OpaqueId:  strings.Split(parts, "/")[1],
		},
	}, nil
}

// CopyMetadata copies all extended attributes from source to target.
// The optional filter function can be used to filter by attribute name, e.g. by checking a prefix
func CopyMetadata(s, t string, filter func(attributeName string) bool) error {
	var attrs []string
	var err error
	if attrs, err = xattr.List(s); err != nil {
		return err
	}
	for i := range attrs {
		if filter == nil || filter(attrs[i]) {
			b, err := xattr.Get(s, attrs[i])
			if err != nil {
				return err
			}
			if err := xattr.Set(t, attrs[i], b); err != nil {
				return err
			}
		}
	}
	return nil
}

// Set an extended attribute key to the given value
func Set(filePath string, key string, val string) error {

	if err := xattr.Set(filePath, key, []byte(val)); err != nil {
		return err
	}
	return nil
}

// SetMultiple allows setting multiple key value pairs at once
// the changes are protected with an flock
func SetMultiple(filePath string, attribs map[string]string) error {

	// h, err := lockedfile.OpenFile(filePath, os.O_WRONLY, 0) // 0? Open File only workn for files ... but we want to lock dirs ... or symlinks
	// or we append .lock to the file and use https://github.com/gofrs/flock
	h, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer h.Close()
	err = unix.Flock(int(h.Fd()), unix.LOCK_EX)
	if err != nil {
		return err
	}
	defer unix.Flock(int(h.Fd()), unix.LOCK_UN)

	for key, val := range attribs {
		if err := xattr.FSet(h, key, []byte(val)); err != nil {
			return err
		}
	}
	return nil
}

// Get an extended attribute value for the given key
func Get(filePath, key string) (string, error) {

	v, err := xattr.Get(filePath, key)
	if err != nil {
		return "", err
	}
	val := string(v)
	return val, nil
}

// GetInt64 reads a string as int64 from the xattrs
func GetInt64(filePath, key string) (int64, error) {
	attr, err := Get(filePath, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(attr, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// All reads all extended attributes for a node
func All(filePath string) (map[string]string, error) {
	attrNames, err := xattr.List(filePath)
	if err != nil {
		return nil, err
	}

	attribs := make(map[string]string, len(attrNames))
	for _, name := range attrNames {
		val, err := xattr.Get(filePath, name)
		if err != nil {
			return nil, err
		}
		attribs[name] = string(val)
	}

	return attribs, nil
}
