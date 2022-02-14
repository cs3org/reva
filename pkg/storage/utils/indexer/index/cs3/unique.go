package cs3

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	idxerrs "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/registry"
	metadata "github.com/cs3org/reva/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/pkg/utils"
)

// Unique are fields for an index of type non_unique.
type Unique struct {
	caseInsensitive bool
	indexBy         string
	typeName        string
	filesDir        string
	indexBaseDir    string
	indexRootDir    string

	tokenManager    token.Manager
	storageProvider provider.ProviderAPIClient
	metadata        *metadata.Storage

	cs3conf *Config
}

func init() {
	registry.IndexConstructorRegistry["cs3"]["unique"] = NewUniqueIndexWithOptions
}

// NewUniqueIndexWithOptions instantiates a new UniqueIndex instance. Init() should be
// called afterward to ensure correct on-disk structure.
func NewUniqueIndexWithOptions(o ...option.Option) index.Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	u := &Unique{
		caseInsensitive: opts.CaseInsensitive,
		indexBy:         opts.IndexBy,
		typeName:        opts.TypeName,
		filesDir:        opts.FilesDir,
		indexBaseDir:    path.Join(opts.DataDir, "index.cs3"),
		indexRootDir:    path.Join(path.Join(opts.DataDir, "index.cs3"), strings.Join([]string{"unique", opts.TypeName, opts.IndexBy}, ".")),
		cs3conf: &Config{
			ProviderAddr: opts.ProviderAddr,
			JWTSecret:    opts.JWTSecret,
			ServiceUser:  opts.ServiceUser,
		},
	}

	return u
}

// Init initializes a unique index.
func (idx *Unique) Init() error {
	tokenManager, err := jwt.New(map[string]interface{}{
		"secret": idx.cs3conf.JWTSecret,
	})
	if err != nil {
		return err
	}
	idx.tokenManager = tokenManager

	client, err := pool.GetStorageProviderServiceClient(idx.cs3conf.ProviderAddr)
	if err != nil {
		return err
	}
	idx.storageProvider = client

	m, err := metadata.NewStorage(idx.cs3conf.ProviderAddr, idx.cs3conf.ServiceUser)
	if err != nil {
		return err
	}
	idx.metadata = m
	if err := idx.metadata.Init(context.Background()); err != nil {
		return err
	}

	if err := idx.makeDirIfNotExists(idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.makeDirIfNotExists(idx.indexRootDir); err != nil {
		return err
	}

	return nil
}

// Lookup exact lookup by value.
func (idx *Unique) Lookup(v string) ([]string, error) {
	if idx.caseInsensitive {
		v = strings.ToLower(v)
	}
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

// Add adds a value to the index, returns the path to the root-document
func (idx *Unique) Add(id, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if idx.caseInsensitive {
		v = strings.ToLower(v)
	}
	newName := path.Join(idx.indexRootDir, v)
	if err := idx.createSymlink(id, newName); err != nil {
		if os.IsExist(err) {
			return "", &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return "", err
	}

	return newName, nil
}

// Remove a value v from an index.
func (idx *Unique) Remove(id string, v string) error {
	if v == "" {
		return nil
	}
	if idx.caseInsensitive {
		v = strings.ToLower(v)
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
func (idx *Unique) Update(id, oldV, newV string) error {
	if idx.caseInsensitive {
		oldV = strings.ToLower(oldV)
		newV = strings.ToLower(newV)
	}

	if err := idx.Remove(id, oldV); err != nil {
		return err
	}

	if _, err := idx.Add(id, newV); err != nil {
		return err
	}

	return nil
}

// Search allows for glob search on the index.
func (idx *Unique) Search(pattern string) ([]string, error) {
	if idx.caseInsensitive {
		pattern = strings.ToLower(pattern)
	}

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
func (idx *Unique) CaseInsensitive() bool {
	return idx.caseInsensitive
}

// IndexBy undocumented.
func (idx *Unique) IndexBy() string {
	return idx.indexBy
}

// TypeName undocumented.
func (idx *Unique) TypeName() string {
	return idx.typeName
}

// FilesDir undocumented.
func (idx *Unique) FilesDir() string {
	return idx.filesDir
}

func (idx *Unique) createSymlink(oldname, newname string) error {
	if _, err := idx.resolveSymlink(newname); err == nil {
		return os.ErrExist
	}

	err := idx.metadata.SimpleUpload(context.Background(), newname, []byte(oldname))
	if err != nil {
		return err
	}

	return nil
}

func (idx *Unique) resolveSymlink(name string) (string, error) {
	b, err := idx.metadata.SimpleDownload(context.Background(), name)
	if err != nil {
		if errors.Is(err, errtypes.NotFound("")) {
			return "", os.ErrNotExist
		}
		return "", err
	}

	return string(b), err
}

func (idx *Unique) makeDirIfNotExists(folder string) error {
	return idx.metadata.MakeDirIfNotExist(context.Background(), &provider.ResourceId{
		StorageId: idx.cs3conf.ServiceUser.Id.OpaqueId,
		OpaqueId:  idx.cs3conf.ServiceUser.Id.OpaqueId,
	}, folder)
}

// Delete deletes the index folder from its storage.
func (idx *Unique) Delete() error {
	return deleteIndexRoot(context.Background(), idx.storageProvider, idx.cs3conf.ServiceUser.Id.OpaqueId, idx.indexRootDir)
}
