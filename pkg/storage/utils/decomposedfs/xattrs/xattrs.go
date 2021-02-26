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

// Declare a list of xattr keys
// TODO the below comment is currently copied from the owncloud driver, revisit
// Currently,extended file attributes have four separated
// namespaces (user, trusted, security and system) followed by a dot.
// A non root user can only manipulate the user. namespace, which is what
// we will use to store ownCloud specific metadata. To prevent name
// collisions with other apps We are going to introduce a sub namespace
// "user.ocis."
const (
	OcisPrefix   string = "user.ocis."
	ParentidAttr string = OcisPrefix + "parentid"
	OwnerIDAttr  string = OcisPrefix + "owner.id"
	OwnerIDPAttr string = OcisPrefix + "owner.idp"
	// the base name of the node
	// updated when the file is renamed or moved
	NameAttr     string = OcisPrefix + "name"
	BlobIDAttr   string = OcisPrefix + "blobid"
	BlobsizeAttr string = OcisPrefix + "blobsize"

	// grantPrefix is the prefix for sharing related extended attributes
	GrantPrefix    string = OcisPrefix + "grant."
	MetadataPrefix string = OcisPrefix + "md."

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

	UserAcePrefix  string = "u:"
	GroupAcePrefix string = "g:"
)
