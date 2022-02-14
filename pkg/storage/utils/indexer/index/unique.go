package index

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	idxerrs "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	metadata "github.com/cs3org/reva/pkg/storage/utils/metadata"
)

// Unique are fields for an index of type non_unique.
type Unique struct {
	caseInsensitive bool
	indexBy         string
	typeName        string
	filesDir        string
	indexBaseDir    string
	indexRootDir    string

	storage metadata.Storage
}

// NewUniqueIndexWithOptions instantiates a new UniqueIndex instance. Init() should be
// called afterward to ensure correct on-disk structure.
func NewUniqueIndexWithOptions(o ...option.Option) Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	u := &Unique{
		caseInsensitive: opts.CaseInsensitive,
		indexBy:         opts.IndexBy,
		typeName:        opts.TypeName,
		filesDir:        opts.FilesDir,
		indexBaseDir:    path.Join(opts.Prefix, "index.cs3"),
		indexRootDir:    path.Join(opts.Prefix, "index.cs3", strings.Join([]string{"unique", opts.TypeName, opts.IndexBy}, ".")),
		storage:         opts.Storage,
	}

	return u
}

// Init initializes a unique index.
func (idx *Unique) Init() error {
	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexRootDir); err != nil {
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
	oldname, err := idx.storage.ResolveSymlink(context.Background(), searchPath)
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
	if err := idx.storage.CreateSymlink(context.Background(), id, newName); err != nil {
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
	_, err := idx.storage.ResolveSymlink(context.Background(), searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return err
	}

	deletePath := path.Join("/", idx.indexRootDir, v)
	return idx.storage.Delete(context.Background(), deletePath)
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

	infos, err := idx.storage.ListContainer(context.Background(), idx.indexRootDir)
	if err != nil {
		return nil, err
	}

	searchPath := idx.indexRootDir
	matches := make([]string, 0)
	for _, i := range infos {
		if found, err := filepath.Match(pattern, path.Base(i.Path)); found {
			if err != nil {
				return nil, err
			}

			oldPath, err := idx.storage.ResolveSymlink(context.Background(), path.Join(searchPath, path.Base(i.Path)))
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

// Delete deletes the index folder from its storage.
func (idx *Unique) Delete() error {
	return idx.storage.Delete(context.Background(), idx.indexRootDir)
}
