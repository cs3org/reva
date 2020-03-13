package ocis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/pkg/errors"
)

// Path implements transformations from filepath to node and back
type Path struct {
	// ocis fs works on top of a dir of uuid nodes
	Root string `mapstructure:"root"`

	// UserLayout wraps the internal path with user information.
	// Example: if conf.Namespace is /ocis/user and received path is /docs
	// and the UserLayout is {{.Username}} the internal path will be:
	// /ocis/user/<username>/docs
	UserLayout string `mapstructure:"user_layout"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`
}

// Resolve takes in a request path or request id and converts it to a NodeInfo
func (pw *Path) Resolve(ctx context.Context, ref *provider.Reference) (*NodeInfo, error) {
	if ref.GetPath() != "" {
		return pw.Wrap(ctx, ref.GetPath())
	}

	if ref.GetId() != nil {
		return pw.WrapID(ctx, ref.GetId())
	}

	// reference is invalid
	return nil, fmt.Errorf("invalid reference %+v", ref)
}

// Wrap converts a filename into a NodeInfo
func (pw *Path) Wrap(ctx context.Context, fn string) (node *NodeInfo, err error) {
	var link, root string
	if fn == "" {
		fn = "/"
	}
	if pw.EnableHome && pw.UserLayout != "" {
		// start at the users root node
		var u *userpb.User

		u, err = getUser(ctx)
		if err != nil {
			err = errors.Wrap(err, "ocisfs: Wrap: no user in ctx and home is enabled")
			return
		}

		layout := templates.WithUser(u, pw.UserLayout)
		root = filepath.Join(pw.Root, "users", layout)

	} else {
		// start at the storage root node
		root = filepath.Join(pw.Root, "nodes/root")
	}

	node, err = pw.ReadRootLink(root)
	// The symlink contains the nodeID
	if err != nil {
		return
	}

	if fn != "/" {
		// we need to walk the path
		segments := strings.Split(strings.TrimLeft(fn, "/"), "/")
		for i := range segments {
			node.ParentID = node.ID
			node.ID = ""
			node.Name = segments[i]

			link, err = os.Readlink(filepath.Join(pw.Root, "nodes", node.ParentID, "children", node.Name))
			if os.IsNotExist(err) {
				node.Exists = false
				// if this is the last segment we can use it as the node name
				if i == len(segments)-1 {
					err = nil
					return
				}

				err = errtypes.NotFound(filepath.Join(pw.Root, "nodes", node.ParentID, "children", node.Name))
				return
			}
			if err != nil {
				err = errors.Wrap(err, "ocisfs: Wrap: readlink error")
				return
			}
			if strings.HasPrefix(link, "../../") {
				node.ID = filepath.Base(link)
			} else {
				err = fmt.Errorf("ocisfs: expected '../../ prefix, got' %+v", link)
				return
			}
		}
	}

	return
}

// WrapID returns the internal path for the id
func (pw *Path) WrapID(ctx context.Context, id *provider.ResourceId) (*NodeInfo, error) {
	if id == nil || id.OpaqueId == "" {
		return nil, fmt.Errorf("invalid resource id %+v", id)
	}
	return &NodeInfo{ID: id.OpaqueId}, nil
}

func (pw *Path) Unwrap(ctx context.Context, ni *NodeInfo) (external string, err error) {
	for err == nil {
		err = pw.FillParentAndName(ni)
		if os.IsNotExist(err) {
			err = nil
			return
		}
		if err != nil {
			err = errors.Wrap(err, "ocisfs: Unwrap: could not fill node")
			return
		}
		external = filepath.Join(ni.Name, external)
		ni.BecomeParent()
	}
	return
}

// FillParentAndName reads the symbolic link and extracts the parent ID and the name of the node if necessary
func (pw *Path) FillParentAndName(node *NodeInfo) (err error) {

	if node == nil || node.ID == "" {
		err = fmt.Errorf("ocisfs: invalid node info '%+v'", node)
	}

	// check if node is already filled
	if node.ParentID != "" && node.Name != "" {
		return
	}

	var link string
	// The parentname symlink looks like `../76455834-769e-412a-8a01-68f265365b79/children/myname.txt`
	link, err = os.Readlink(filepath.Join(pw.Root, "nodes", node.ID, "parentname"))
	if err != nil {
		return
	}

	// check the link follows the correct schema
	// TODO count slashes
	if strings.HasPrefix(link, "../") {
		node.Name = filepath.Base(link)
		node.ParentID = filepath.Base(filepath.Dir(filepath.Dir(link)))
		node.Exists = true
	} else {
		err = fmt.Errorf("ocisfs: expected '../' prefix, got '%+v'", link)
		return
	}
	return
}

// ReadRootLink reads the symbolic link and extracts the node id
func (pw *Path) ReadRootLink(root string) (node *NodeInfo, err error) {

	// A root symlink looks like `../nodes/76455834-769e-412a-8a01-68f265365b79`
	link, err := os.Readlink(root)
	if os.IsNotExist(err) {
		err = errtypes.NotFound(root)
		return
	}

	// extract the nodeID
	if strings.HasPrefix(link, "../nodes/") {
		node = &NodeInfo{
			ID:     filepath.Base(link),
			Exists: true,
		}
	} else {
		err = fmt.Errorf("ocisfs: expected '../nodes/ prefix, got' %+v", link)
	}
	return
}
