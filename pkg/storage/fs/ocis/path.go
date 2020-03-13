package ocis

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/templates"
	"github.com/pkg/errors"
)

type Path struct {
	// ocis fs works on top of a dir of uuid nodes
	DataDirectory string `mapstructure:"data_directory"`

	// UserLayout wraps the internal path with user information.
	// Example: if conf.Namespace is /ocis/user and received path is /docs
	// and the UserLayout is {{.Username}} the internal path will be:
	// /ocis/user/<username>/docs
	UserLayout string `mapstructure:"user_layout"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`
}

// Resolve takes in a request path or request id and converts it to an internal path.
func (pw *Path) Resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return pw.Wrap(ctx, ref.GetPath())
	}

	if ref.GetId() != nil {
		return pw.WrapID(ctx, ref.GetId())
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v", ref)
}

func (pw *Path) Wrap(ctx context.Context, fn string) (internal string, err error) {
	var link, nodeID, root string
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
		root = path.Join(pw.DataDirectory, "users", layout)

	} else {
		// start at the storage root node
		root = path.Join(pw.DataDirectory, "nodes", "root")
	}

	// The symlink contains the nodeID
	link, err = os.Readlink(root)
	if os.IsNotExist(err) {
		err = errtypes.NotFound(fn)
		return
	}
	if err != nil {
		err = errors.Wrap(err, "ocisfs: Wrap: readlink error")
		return
	}

	// extract the nodeID
	if strings.HasPrefix(link, "../nodes/") { // TODO does not take into account the template
		nodeID = link[9:]
		if strings.Contains(nodeID, "/") {
			err = fmt.Errorf("ocisfs: node id must not contain / %+v", nodeID) // TODO allow this to distribute nodeids over multiple folders
			return
		}
	} else {
		err = fmt.Errorf("ocisfs: expected '../nodes/ prefix, got' %+v", link)
		return
	}

	if fn != "/" {
		// we need to walk the path
		segments := strings.Split(strings.TrimLeft(fn, "/"), "/")
		for i := range segments {
			link, err = os.Readlink(path.Join(pw.DataDirectory, "nodes", nodeID, "children", segments[i]))
			if os.IsNotExist(err) {
				err = errtypes.NotFound(path.Join(pw.DataDirectory, "nodes", nodeID, "children", segments[i]))
				return
			}
			if err != nil {
				err = errors.Wrap(err, "ocisfs: Wrap: readlink error")
				return
			}
			if strings.HasPrefix(link, "../../") {
				nodeID = link[6:]
				if strings.Contains(nodeID, "/") {
					err = fmt.Errorf("ocisfs: node id must not contain / %+v", nodeID)
					return
				}
			} else {
				err = fmt.Errorf("ocisfs: expected '../../ prefix, got' %+v", link)
				return
			}
		}
	}

	internal = path.Join(pw.DataDirectory, "nodes", nodeID)
	return
}

// WrapID returns the internal path for the id
func (pw *Path) WrapID(ctx context.Context, id *provider.ResourceId) (string, error) {
	if id == nil || id.GetOpaqueId() == "" {
		return "", fmt.Errorf("invalid resource id %+v", id)
	}
	return path.Join(pw.DataDirectory, "nodes", id.GetOpaqueId()), nil
}

func (pw *Path) Unwrap(ctx context.Context, internal string) (external string, err error) {
	var link string
	for err == nil {
		link, err = os.Readlink(path.Join(internal, "parentname"))
		if os.IsNotExist(err) {
			err = nil
			return
		}
		if err != nil {
			err = errors.Wrap(err, "ocisfs: getNode: readlink error")
			return
		}
		parentID := path.Base(path.Dir(path.Dir(link)))
		internal = path.Join(pw.DataDirectory, "nodes", parentID)
		external = path.Join(path.Base(link), external)
	}
	return
}

// ReadParentName reads the symbolic link and extracts the parnetNodeID and the name of the child
func (pw *Path) ReadParentName(ctx context.Context, internal string) (parentNodeID string, name string, err error) {

	// The parentname symlink looks like `../76455834-769e-412a-8a01-68f265365b79/children/myname.txt`
	link, err := os.Readlink(path.Join(internal, "parentname"))
	if os.IsNotExist(err) {
		err = errtypes.NotFound(internal)
		return
	}

	// check the link follows the correct schema
	// TODO count slashes
	if strings.HasPrefix(link, "../") {
		name = path.Base(link)
		parentNodeID = path.Base(path.Dir(path.Dir(link)))
	} else {
		err = fmt.Errorf("ocisfs: expected '../' prefix, got '%+v'", link)
		return
	}
	return
}
