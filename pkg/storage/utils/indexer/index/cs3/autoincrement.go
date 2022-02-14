package cs3

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	idxerrs "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/registry"
	metadata "github.com/cs3org/reva/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/pkg/utils"
)

// Autoincrement are fields for an index of type autoincrement.
type Autoincrement struct {
	indexBy      string
	typeName     string
	filesDir     string
	indexBaseDir string
	indexRootDir string

	metadata *metadata.Storage

	cs3conf *Config
	bound   *option.Bound
}

func init() {
	registry.IndexConstructorRegistry["cs3"]["autoincrement"] = NewAutoincrementIndex
}

// NewAutoincrementIndex instantiates a new AutoincrementIndex instance.
func NewAutoincrementIndex(o ...option.Option) index.Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	u := &Autoincrement{
		indexBy:      opts.IndexBy,
		typeName:     opts.TypeName,
		filesDir:     opts.FilesDir,
		bound:        opts.Bound,
		indexBaseDir: path.Join(opts.DataDir, "index.cs3"),
		indexRootDir: path.Join(path.Join(opts.DataDir, "index.cs3"), strings.Join([]string{"autoincrement", opts.TypeName, opts.IndexBy}, ".")),
		cs3conf: &Config{
			ProviderAddr: opts.ProviderAddr,
			JWTSecret:    opts.JWTSecret,
			ServiceUser:  opts.ServiceUser,
		},
	}

	return u
}

// Init initializes an autoincrement index.
func (idx *Autoincrement) Init() error {
	if err := idx.makeDirIfNotExists(idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.makeDirIfNotExists(idx.indexRootDir); err != nil {
		return err
	}

	return nil
}

// Lookup exact lookup by value.
func (idx *Autoincrement) Lookup(v string) ([]string, error) {
	searchPath := path.Join(idx.indexRootDir, v)
	oldname, err := idx.resolveSymlink(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return nil, err
	}

	return []string{oldname}, nil
}

// Add a new value to the index.
func (idx *Autoincrement) Add(id, v string) (string, error) {
	var newName string
	if v == "" {
		next, err := idx.next()
		if err != nil {
			return "", err
		}
		newName = path.Join(idx.indexRootDir, strconv.Itoa(next))
	} else {
		newName = path.Join(idx.indexRootDir, v)
	}
	if err := idx.createSymlink(id, newName); err != nil {
		if os.IsExist(err) {
			return "", &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return "", err
	}

	return newName, nil
}

// Remove a value v from an index.
func (idx *Autoincrement) Remove(id string, v string) error {
	if v == "" {
		return nil
	}
	searchPath := path.Join(idx.indexRootDir, v)
	_, err := idx.resolveSymlink(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return err
	}

	deletePath := path.Join("/", idx.indexRootDir, v)
	resp, err := idx.storageProvider.Delete(context.Background(), &provider.DeleteRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: idx.cs3conf.ServiceUser.Id.OpaqueId,
				OpaqueId:  idx.cs3conf.ServiceUser.Id.OpaqueId,
			},
			Path: utils.MakeRelativePath(deletePath),
		},
	})

	if err != nil {
		return err
	}

	// TODO Handle other error codes?
	if resp.Status.Code == v1beta11.Code_CODE_NOT_FOUND {
		return &idxerrs.NotFoundErr{}
	}

	return err
}

// Update index from <oldV> to <newV>.
func (idx *Autoincrement) Update(id, oldV, newV string) error {
	if err := idx.Remove(id, oldV); err != nil {
		return err
	}

	if _, err := idx.Add(id, newV); err != nil {
		return err
	}

	return nil
}

// Search allows for glob search on the index.
func (idx *Autoincrement) Search(pattern string) ([]string, error) {
	res, err := idx.storageProvider.ListContainer(context.Background(), &provider.ListContainerRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: idx.cs3conf.ServiceUser.Id.OpaqueId,
				OpaqueId:  idx.cs3conf.ServiceUser.Id.OpaqueId,
			},
			Path: utils.MakeRelativePath(idx.indexRootDir),
		},
	})

	if err != nil {
		return nil, err
	}

	searchPath := idx.indexRootDir
	matches := make([]string, 0)
	for _, i := range res.GetInfos() {
		if found, err := filepath.Match(pattern, path.Base(i.Path)); found {
			if err != nil {
				return nil, err
			}

			oldPath, err := idx.resolveSymlink(path.Join(searchPath, path.Base(i.Path)))
			if err != nil {
				return nil, err
			}
			matches = append(matches, oldPath)
		}
	}

	return matches, nil
}

// CaseInsensitive undocumented.
func (idx *Autoincrement) CaseInsensitive() bool {
	return false
}

// IndexBy undocumented.
func (idx *Autoincrement) IndexBy() string {
	return idx.indexBy
}

// TypeName undocumented.
func (idx *Autoincrement) TypeName() string {
	return idx.typeName
}

// FilesDir  undocumented.
func (idx *Autoincrement) FilesDir() string {
	return idx.filesDir
}

func (idx *Autoincrement) createSymlink(oldname, newname string) error {
	if _, err := idx.resolveSymlink(newname); err == nil {
		return os.ErrExist
	}

	err := idx.metadata.SimpleUpload(context.Background(), newname, []byte(oldname))
	if err != nil {
		return err
	}
	return nil
}

func (idx *Autoincrement) resolveSymlink(name string) (string, error) {
	b, err := idx.metadata.SimpleDownload(context.Background(), name)
	if err != nil {
		if errors.Is(err, errtypes.NotFound("")) {
			return "", os.ErrNotExist
		}
		return "", err
	}

	return string(b), err
}

func (idx *Autoincrement) makeDirIfNotExists(folder string) error {
	return idx.metadata.MakeDirIfNotExist(context.Background(), &provider.ResourceId{
		StorageId: idx.cs3conf.ServiceUser.Id.OpaqueId,
		OpaqueId:  idx.cs3conf.ServiceUser.Id.OpaqueId,
	}, folder)
}

func (idx *Autoincrement) next() (int, error) {
	res, err := idx.storageProvider.ListContainer(context.Background(), &provider.ListContainerRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: idx.cs3conf.ServiceUser.Id.OpaqueId,
				OpaqueId:  idx.cs3conf.ServiceUser.Id.OpaqueId,
			},
			Path: utils.MakeRelativePath(idx.indexRootDir),
		},
	})

	if err != nil {
		return -1, err
	}

	if len(res.GetInfos()) == 0 {
		return 0, nil
	}

	infos := res.GetInfos()
	sort.Slice(infos, func(i, j int) bool {
		a, _ := strconv.Atoi(path.Base(infos[i].Path))
		b, _ := strconv.Atoi(path.Base(infos[j].Path))
		return a < b
	})

	latest, err := strconv.Atoi(path.Base(infos[len(infos)-1].Path)) // would returning a string be a better interface?
	if err != nil {
		return -1, err
	}

	if int64(latest) < idx.bound.Lower {
		return int(idx.bound.Lower), nil
	}

	return latest + 1, nil
}

// Delete deletes the index folder from its storage.
func (idx *Autoincrement) Delete() error {
	return deleteIndexRoot(context.Background(), idx.storageProvider, idx.cs3conf.ServiceUser.Id.OpaqueId, idx.indexRootDir)
}
