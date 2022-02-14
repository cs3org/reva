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

// NonUnique are fields for an index of type non_unique.
type NonUnique struct {
	caseInsensitive bool
	indexBy         string
	typeName        string
	filesDir        string
	indexBaseDir    string
	indexRootDir    string

	storage metadata.Storage
}

// NewNonUniqueIndexWithOptions instantiates a new NonUniqueIndex instance.
// /tmp/ocis/accounts/index.cs3/Pets/Bro*
// ├── Brown/
// │   └── rebef-123 -> /tmp/testfiles-395764020/pets/rebef-123
// ├── Green/
// │    ├── goefe-789 -> /tmp/testfiles-395764020/pets/goefe-789
// │    └── xadaf-189 -> /tmp/testfiles-395764020/pets/xadaf-189
// └── White/
//     └── wefwe-456 -> /tmp/testfiles-395764020/pets/wefwe-456
func NewNonUniqueIndexWithOptions(o ...option.Option) Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	return &NonUnique{
		caseInsensitive: opts.CaseInsensitive,
		indexBy:         opts.IndexBy,
		typeName:        opts.TypeName,
		filesDir:        opts.FilesDir,
		indexBaseDir:    path.Join(opts.Prefix, "index.cs3"),
		indexRootDir:    path.Join(opts.Prefix, "index.cs3", strings.Join([]string{"non_unique", opts.TypeName, opts.IndexBy}, ".")),
	}
}

// Init initializes a non_unique index.
func (idx *NonUnique) Init() error {
	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexRootDir); err != nil {
		return err
	}

	return nil
}

// Lookup exact lookup by value.
func (idx *NonUnique) Lookup(v string) ([]string, error) {
	if idx.caseInsensitive {
		v = strings.ToLower(v)
	}
	var matches = make([]string, 0)
	infos, err := idx.storage.ListContainer(context.Background(), path.Join("/", idx.indexRootDir, v))

	if err != nil {
		return nil, err
	}

	for _, info := range infos {
		matches = append(matches, path.Base(info.Path))
	}

	return matches, nil
}

// Add a new value to the index.
func (idx *NonUnique) Add(id, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if idx.caseInsensitive {
		v = strings.ToLower(v)
	}

	newName := path.Join(idx.indexRootDir, v)
	if err := idx.storage.MakeDirIfNotExist(context.Background(), newName); err != nil {
		return "", err
	}

	if err := idx.storage.CreateSymlink(context.Background(), id, path.Join(newName, id)); err != nil {
		if os.IsExist(err) {
			return "", &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return "", err
	}

	return newName, nil
}

// Remove a value v from an index.
func (idx *NonUnique) Remove(id string, v string) error {
	if v == "" {
		return nil
	}
	if idx.caseInsensitive {
		v = strings.ToLower(v)
	}

	deletePath := path.Join("/", idx.indexRootDir, v, id)
	err := idx.storage.Delete(context.Background(), deletePath)
	if err != nil {
		return err
	}

	toStat := path.Join("/", idx.indexRootDir, v)
	infos, err := idx.storage.ListContainer(context.Background(), toStat)
	if err != nil {
		return err
	}

	if len(infos) == 0 {
		deletePath = path.Join("/", idx.indexRootDir, v)
		err := idx.storage.Delete(context.Background(), deletePath)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update index from <oldV> to <newV>.
func (idx *NonUnique) Update(id, oldV, newV string) error {
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
func (idx *NonUnique) Search(pattern string) ([]string, error) {
	if idx.caseInsensitive {
		pattern = strings.ToLower(pattern)
	}

	foldersMatched := make([]string, 0)
	matches := make([]string, 0)
	infos, err := idx.storage.ListContainer(context.Background(), idx.indexRootDir)

	if err != nil {
		return nil, err
	}

	for _, i := range infos {
		if found, err := filepath.Match(pattern, path.Base(i.Path)); found {
			if err != nil {
				return nil, err
			}

			foldersMatched = append(foldersMatched, i.Path)
		}
	}

	for i := range foldersMatched {
		infos, _ := idx.storage.ListContainer(context.Background(), foldersMatched[i])

		for _, info := range infos {
			matches = append(matches, path.Base(info.Path))
		}
	}

	return matches, nil
}

// CaseInsensitive undocumented.
func (idx *NonUnique) CaseInsensitive() bool {
	return idx.caseInsensitive
}

// IndexBy undocumented.
func (idx *NonUnique) IndexBy() string {
	return idx.indexBy
}

// TypeName undocumented.
func (idx *NonUnique) TypeName() string {
	return idx.typeName
}

// FilesDir  undocumented.
func (idx *NonUnique) FilesDir() string {
	return idx.filesDir
}

// Delete deletes the index folder from its storage.
func (idx *NonUnique) Delete() error {
	return idx.storage.Delete(context.Background(), idx.indexRootDir)
}
