package ocis

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

type Tree struct {
	pw            PathWrapper
	DataDirectory string
}

func NewTree(pw PathWrapper, dataDirectory string) (TreePersistence, error) {
	return &Tree{
		pw:            pw,
		DataDirectory: dataDirectory,
	}, nil
}

func (fs *Tree) GetMD(ctx context.Context, internal string) (os.FileInfo, error) {
	md, err := os.Stat(internal)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(internal)
		}
		return nil, errors.Wrap(err, "tree: error stating "+internal)
	}

	return md, nil
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *Tree) GetPathByID(ctx context.Context, id *provider.ResourceId) (relativeExternalPath string, err error) {
	var internal string
	internal, err = fs.pw.WrapID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = fs.pw.Unwrap(ctx, path.Join("/", internal))
	if !strings.HasPrefix(relativeExternalPath, fs.DataDirectory) {
		return "", fmt.Errorf("ocisfs: GetPathByID wrong prefix")
	}

	relativeExternalPath = strings.TrimPrefix(relativeExternalPath, fs.DataDirectory)
	return
}

func (fs *Tree) CreateDir(ctx context.Context, internal string, newName string) (err error) {

	internalChild := path.Join(internal, "children", newName)
	_, err = os.Stat(internalChild)
	if err == nil { // child already exists
		return nil
	}

	// create a directory node (with children subfolder)
	nodeID := uuid.Must(uuid.NewV4()).String()

	nodePath := path.Join(fs.DataDirectory, "nodes", nodeID)

	err = os.MkdirAll(path.Join(nodePath, "children"), 0700)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not create node dir")
	}

	// create back link
	// we are not only linking back to the parent, but also to the filename
	err = os.Symlink("../"+path.Base(internal)+"/children/"+newName, path.Join(nodePath, "parentname"))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not symlink parent node")
	}

	// link child name to node
	err = os.Symlink("../../"+nodeID, internalChild)
	if err != nil {
		return
	}
	return fs.Propagate(ctx, nodePath)
}

func (fs *Tree) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("operation not supported: CreateReference")
}

func (fs *Tree) Move(ctx context.Context, oldInternal string, newInternal string) (err error) {
	oldParentID, oldName, err = fs.pw.ReadParentName(ctx, oldInternal)
	if err != nil {
		return err
	}
	newParentID, newName, err = fs.pw.ReadParentName(ctx, oldInternal)
	if err != nil {
		return err
	}
	if err := os.Rename(oldInternal, newInternal); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldInternal+" to "+newInternal)
	}
	return errtypes.NotSupported("operation not supported: Move")
}

func (fs *Tree) ListFolder(ctx context.Context, internal string) ([]os.FileInfo, error) {

	children := path.Join(internal, "children")

	mds, err := ioutil.ReadDir(children)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(children)
		}
		return nil, errors.Wrap(err, "tree: error listing "+children)
	}

	return mds, nil
}

func (fs *Tree) Delete(ctx context.Context, internal string) (err error) {
	// resolve the parent

	// The nodes parentname symlink contains the nodeID and the file name
	link, err := os.Readlink(path.Join(internal, "parentname"))
	if os.IsNotExist(err) {
		err = errtypes.NotFound(internal)
		return
	}

	// remove child entry from dir

	childName := path.Base(link)
	parentNodeID := path.Base(path.Dir(path.Dir(link)))
	os.Remove(path.Join(fs.DataDirectory, "nodes", parentNodeID, "children", childName))

	nodeID := path.Base(internal)

	src := path.Join(fs.DataDirectory, "nodes", nodeID)
	trashpath := path.Join(fs.DataDirectory, "trash/files", nodeID)
	err = os.Rename(src, trashpath)
	if err != nil {
		return
	}

	// write a trash info ... slightly violating the freedesktop trash spec
	t := time.Now()
	// TODO store the original Path
	info := []byte("[Trash Info]\nParentID=" + parentNodeID + "\nDeletionDate=" + t.Format(time.RFC3339))
	infoPath := path.Join(fs.DataDirectory, "trash/info", nodeID+".trashinfo")
	err = ioutil.WriteFile(infoPath, info, 0700)
	if err != nil {
		return
	}
	return fs.Propagate(ctx, path.Join(fs.DataDirectory, "nodes", parentNodeID))
}

func (fs *Tree) Propagate(ctx context.Context, internal string) (err error) {
	// generate an etag
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	// store in extended attribute
	etag := hex.EncodeToString(bytes)
	var link string
	for err == nil {
		if err := xattr.Set(internal, "user.ocis.etag", []byte(etag)); err != nil {
			log.Error().Err(err).Msg("error storing file id")
		}
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
		internal = path.Join(fs.DataDirectory, "nodes", parentID)
	}
	return
}
