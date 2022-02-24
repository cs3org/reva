package disk

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	idxerrs "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"

	"github.com/cs3org/reva/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/registry"
)

// Autoincrement are fields for an index of type autoincrement.
type Autoincrement struct {
	indexBy      string
	typeName     string
	filesDir     string
	indexBaseDir string
	indexRootDir string

	bound *option.Bound
}

func init() {
	registry.IndexConstructorRegistry["disk"]["autoincrement"] = NewAutoincrementIndex
}

// NewAutoincrementIndex instantiates a new AutoincrementIndex instance. Init() MUST be called upon instantiation.
func NewAutoincrementIndex(o ...option.Option) index.Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	if opts.Entity == nil {
		panic("invalid autoincrement index: configured without entity")
	}

	k, err := getKind(opts.Entity, opts.IndexBy)
	if !isValidKind(k) || err != nil {
		panic("invalid autoincrement index: configured on non-numeric field")
	}

	return &Autoincrement{
		indexBy:      opts.IndexBy,
		typeName:     opts.TypeName,
		filesDir:     opts.FilesDir,
		bound:        opts.Bound,
		indexBaseDir: path.Join(opts.DataDir, "index.disk"),
		indexRootDir: path.Join(path.Join(opts.DataDir, "index.disk"), strings.Join([]string{"autoincrement", opts.TypeName, opts.IndexBy}, ".")),
	}
}

// Init initializes an autoincrement index.
func (idx *Autoincrement) Init() error {
	if _, err := os.Stat(idx.filesDir); err != nil {
		return err
	}

	if err := os.MkdirAll(idx.indexRootDir, 0777); err != nil {
		return err
	}

	return nil
}

// Lookup exact lookup by value.
func (idx *Autoincrement) Lookup(v string) ([]string, error) {
	searchPath := path.Join(idx.indexRootDir, v)
	if err := isValidSymlink(searchPath); err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
		}

		return nil, err
	}

	p, err := os.Readlink(searchPath)
	if err != nil {
		return []string{}, nil
	}

	return []string{p}, err
}

// Add a new value to the index.
func (idx *Autoincrement) Add(id, v string) (string, error) {
	nextID, err := idx.next()
	if err != nil {
		return "", err
	}
	oldName := filepath.Join(idx.filesDir, id)
	var newName string
	if v == "" {
		newName = filepath.Join(idx.indexRootDir, strconv.Itoa(nextID))
	} else {
		newName = filepath.Join(idx.indexRootDir, v)
	}
	err = os.Symlink(oldName, newName)
	if errors.Is(err, os.ErrExist) {
		return "", &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, Key: idx.indexBy, Value: v}
	}

	return newName, err
}

// Remove a value v from an index.
func (idx *Autoincrement) Remove(id string, v string) error {
	if v == "" {
		return nil
	}
	searchPath := path.Join(idx.indexRootDir, v)
	return os.Remove(searchPath)
}

// Update index from <oldV> to <newV>.
func (idx *Autoincrement) Update(id, oldV, newV string) error {
	oldPath := path.Join(idx.indexRootDir, oldV)
	if err := isValidSymlink(oldPath); err != nil {
		if os.IsNotExist(err) {
			return &idxerrs.NotFoundErr{TypeName: idx.TypeName(), Key: idx.IndexBy(), Value: oldV}
		}

		return err
	}

	newPath := path.Join(idx.indexRootDir, newV)
	err := isValidSymlink(newPath)
	if err == nil {
		return &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, Key: idx.indexBy, Value: newV}
	}

	if os.IsNotExist(err) {
		err = os.Rename(oldPath, newPath)
	}

	return err
}

// Search allows for glob search on the index.
func (idx *Autoincrement) Search(pattern string) ([]string, error) {
	paths, err := filepath.Glob(path.Join(idx.indexRootDir, pattern))
	if err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, &idxerrs.NotFoundErr{TypeName: idx.typeName, Key: idx.indexBy, Value: pattern}
	}

	res := make([]string, 0)
	for _, p := range paths {
		if err := isValidSymlink(p); err != nil {
			return nil, err
		}

		src, err := os.Readlink(p)
		if err != nil {
			return nil, err
		}

		res = append(res, src)
	}

	return res, nil
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

func (idx *Autoincrement) next() (int, error) {
	files, err := readDir(idx.indexRootDir)
	if err != nil {
		return -1, err
	}

	if len(files) == 0 {
		return int(idx.bound.Lower), nil
	}

	latest, err := lastValueFromTree(files)
	if err != nil {
		return -1, err
	}

	if int64(latest) < idx.bound.Lower {
		return int(idx.bound.Lower), nil
	}

	return latest + 1, nil
}

// Delete deletes the index root folder from the configured storage.
func (idx *Autoincrement) Delete() error {
	return os.RemoveAll(idx.indexRootDir)
}

func lastValueFromTree(files []os.FileInfo) (int, error) {
	latest, err := strconv.Atoi(path.Base(files[len(files)-1].Name()))
	if err != nil {
		return -1, err
	}
	return latest, nil
}
