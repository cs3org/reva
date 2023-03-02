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

package lookup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/gofrs/flock"
	"github.com/pkg/errors"
)

// Lookup implements transformations from filepath to node and back
type Lookup struct {
	Options *options.Options

	metadataBackend metadata.Backend
}

func New(b metadata.Backend, o *options.Options) *Lookup {
	return &Lookup{
		Options:         o,
		metadataBackend: b,
	}
}

func (lu *Lookup) MetadataBackend() metadata.Backend {
	return lu.metadataBackend
}

// ReadBlobSizeAttr reads the blobsize from the xattrs
func (lu *Lookup) ReadBlobSizeAttr(path string) (int64, error) {
	attr, err := lu.metadataBackend.Get(path, prefixes.BlobsizeAttr)
	if err != nil {
		return 0, errors.Wrapf(err, "error reading blobsize xattr")
	}
	blobSize, err := strconv.ParseInt(attr, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid blobsize xattr format")
	}
	return blobSize, nil
}

// ReadBlobIDAttr reads the blobsize from the xattrs
func (lu *Lookup) ReadBlobIDAttr(path string) (string, error) {
	attr, err := lu.metadataBackend.Get(path, prefixes.BlobIDAttr)
	if err != nil {
		return "", errors.Wrapf(err, "error reading blobid xattr")
	}
	return attr, nil
}

// TypeFromPath returns the type of the node at the given path
func (lu *Lookup) TypeFromPath(path string) provider.ResourceType {
	// Try to read from xattrs
	typeAttr, err := lu.metadataBackend.Get(path, prefixes.TypeAttr)
	t := provider.ResourceType_RESOURCE_TYPE_INVALID
	if err == nil {
		typeInt, err := strconv.ParseInt(typeAttr, 10, 32)
		if err != nil {
			return t
		}
		return provider.ResourceType(typeInt)
	}

	// Fall back to checking on disk
	fi, err := os.Lstat(path)
	if err != nil {
		return t
	}

	switch {
	case fi.IsDir():
		if _, err = lu.metadataBackend.Get(path, prefixes.ReferenceAttr); err == nil {
			t = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		} else {
			t = provider.ResourceType_RESOURCE_TYPE_CONTAINER
		}
	case fi.Mode().IsRegular():
		t = provider.ResourceType_RESOURCE_TYPE_FILE
	case fi.Mode()&os.ModeSymlink != 0:
		t = provider.ResourceType_RESOURCE_TYPE_SYMLINK
		// TODO reference using ext attr on a symlink
		// nodeType = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	}
	return t
}

// NodeFromResource takes in a request path or request id and converts it to a Node
func (lu *Lookup) NodeFromResource(ctx context.Context, ref *provider.Reference) (*node.Node, error) {
	if ref.ResourceId != nil {
		// check if a storage space reference is used
		// currently, the decomposed fs uses the root node id as the space id
		n, err := lu.NodeFromID(ctx, ref.ResourceId)
		if err != nil {
			return nil, err
		}
		// is this a relative reference?
		if ref.Path != "" {
			p := filepath.Clean(ref.Path)
			if p != "." && p != "/" {
				// walk the relative path
				n, err = lu.WalkPath(ctx, n, p, false, func(ctx context.Context, n *node.Node) error { return nil })
				if err != nil {
					return nil, err
				}
				n.SpaceID = ref.ResourceId.SpaceId
			}
		}
		return n, nil
	}

	// reference is invalid
	return nil, fmt.Errorf("invalid reference %+v. resource_id must be set", ref)
}

// NodeFromID returns the internal path for the id
func (lu *Lookup) NodeFromID(ctx context.Context, id *provider.ResourceId) (n *node.Node, err error) {
	if id == nil {
		return nil, fmt.Errorf("invalid resource id %+v", id)
	}
	if id.OpaqueId == "" {
		// The Resource references the root of a space
		return lu.NodeFromSpaceID(ctx, id)
	}
	return node.ReadNode(ctx, lu, id.SpaceId, id.OpaqueId, false)
}

// Pathify segments the beginning of a string into depth segments of width length
// Pathify("aabbccdd", 3, 1) will return "a/a/b/bccdd"
func Pathify(id string, depth, width int) string {
	b := strings.Builder{}
	i := 0
	for ; i < depth; i++ {
		if len(id) <= i*width+width {
			break
		}
		b.WriteString(id[i*width : i*width+width])
		b.WriteRune(filepath.Separator)
	}
	b.WriteString(id[i*width:])
	return b.String()
}

// NodeFromSpaceID converts a resource id without an opaque id into a Node
func (lu *Lookup) NodeFromSpaceID(ctx context.Context, id *provider.ResourceId) (n *node.Node, err error) {
	node, err := node.ReadNode(ctx, lu, id.SpaceId, id.OpaqueId, false)
	if err != nil {
		return nil, err
	}

	node.SpaceRoot = node
	return node, nil
}

// Path returns the path for node
func (lu *Lookup) Path(ctx context.Context, n *node.Node, hasPermission node.PermissionFunc) (p string, err error) {
	root := n.SpaceRoot
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

		if !hasPermission(n) {
			break
		}
	}
	p = filepath.Join("/", p)
	return
}

// WalkPath calls n.Child(segment) on every path segment in p starting at the node r.
// If a function f is given it will be executed for every segment node, but not the root node r.
// If followReferences is given the current visited reference node is replaced by the referenced node.
func (lu *Lookup) WalkPath(ctx context.Context, r *node.Node, p string, followReferences bool, f func(ctx context.Context, n *node.Node) error) (*node.Node, error) {
	segments := strings.Split(strings.Trim(p, "/"), "/")
	var err error
	for i := range segments {
		if r, err = r.Child(ctx, segments[i]); err != nil {
			return r, err
		}

		if followReferences {
			if attrBytes, err := r.Xattr(prefixes.ReferenceAttr); err == nil {
				realNodeID := attrBytes
				ref, err := refFromCS3([]byte(realNodeID))
				if err != nil {
					return nil, err
				}

				r, err = lu.NodeFromID(ctx, ref.ResourceId)
				if err != nil {
					return nil, err
				}
			}
		}
		if r.IsSpaceRoot() {
			r.SpaceRoot = r
		}

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

// InternalRoot returns the internal storage root directory
func (lu *Lookup) InternalRoot() string {
	return lu.Options.Root
}

// InternalPath returns the internal path for a given ID
func (lu *Lookup) InternalPath(spaceID, nodeID string) string {
	return filepath.Join(lu.Options.Root, "spaces", Pathify(spaceID, 1, 2), "nodes", Pathify(nodeID, 4, 2))
}

// // ReferenceFromAttr returns a CS3 reference from xattr of a node.
// // Supported formats are: "cs3:storageid/nodeid"
// func ReferenceFromAttr(b []byte) (*provider.Reference, error) {
// 	return refFromCS3(b)
// }

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
// For the source file, a shared lock is acquired.
// NOTE: target resource is not locked! You need to acquire a write lock on the target additionally
func (lu *Lookup) CopyMetadata(src, target string, filter func(attributeName string) bool) (err error) {
	var readLock *flock.Flock

	// Acquire a read log on the source node
	readLock, err = filelocks.AcquireReadLock(src)

	if err != nil {
		return errors.Wrap(err, "xattrs: Unable to lock source to read")
	}
	defer func() {
		rerr := filelocks.ReleaseLock(readLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	return lu.CopyMetadataWithSourceLock(src, target, filter, readLock)
}

// CopyMetadataWithSourceLock copies all extended attributes from source to target.
// The optional filter function can be used to filter by attribute name, e.g. by checking a prefix
// For the source file, a shared lock is acquired.
// NOTE: target resource is not locked! You need to acquire a write lock on the target additionally
func (lu *Lookup) CopyMetadataWithSourceLock(src, target string, filter func(attributeName string) bool, readLock *flock.Flock) (err error) {
	switch {
	case readLock == nil:
		return errors.New("no lock provided")
	case readLock.Path() != filelocks.FlockFile(src):
		return errors.New("lockpath does not match filepath")
	case !readLock.Locked() && !readLock.RLocked(): // we need either a read or a write lock
		return errors.New("not locked")
	}

	// both locks are established. Copy.
	var attrNameList []string
	if attrNameList, err = lu.metadataBackend.List(src); err != nil {
		return errors.Wrap(err, "Can not get xattr listing on src")
	}

	// error handling: We count errors of reads or writes of xattrs.
	// if there were any read or write errors an error is returned.
	var (
		xerrs = 0
		xerr  error
	)
	for idx := range attrNameList {
		attrName := attrNameList[idx]
		if filter == nil || filter(attrName) {
			var attrVal string
			if attrVal, xerr = lu.metadataBackend.Get(src, attrName); xerr != nil {
				xerrs++
			}
			if xerr = lu.metadataBackend.Set(target, attrName, attrVal); xerr != nil {
				xerrs++
			}
		}
	}
	if xerrs > 0 {
		err = errors.Wrap(xerr, "failed to copy all xattrs, last error returned")
	}

	return err
}
