// Copyright 2018-2022 CERN
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

package index

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	idxerrs "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	metadata "github.com/cs3org/reva/pkg/storage/utils/metadata"
)

// Autoincrement are fields for an index of type autoincrement.
type Autoincrement struct {
	indexBy      option.IndexBy
	typeName     string
	filesDir     string
	indexBaseDir string
	indexRootDir string

	bound   *option.Bound
	storage metadata.Storage
}

// NewAutoincrementIndex instantiates a new AutoincrementIndex instance.
func NewAutoincrementIndex(storage metadata.Storage, o ...option.Option) Index {
	opts := &option.Options{}
	for _, opt := range o {
		opt(opts)
	}

	u := &Autoincrement{
		storage:      storage,
		indexBy:      opts.IndexBy,
		typeName:     opts.TypeName,
		filesDir:     opts.FilesDir,
		bound:        opts.Bound,
		indexBaseDir: path.Join(opts.Prefix, "index."+storage.Backend()),
		indexRootDir: path.Join(opts.Prefix, "index."+storage.Backend(), strings.Join([]string{"autoincrement", opts.TypeName, opts.IndexBy.String()}, ".")),
	}

	return u
}

// Init initializes an autoincrement index.
func (idx *Autoincrement) Init() error {
	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.storage.MakeDirIfNotExist(context.Background(), idx.indexRootDir); err != nil {
		return err
	}
	return nil
}

// Lookup exact lookup by value.
func (idx *Autoincrement) Lookup(v string) ([]string, error) {
	searchPath := path.Join(idx.indexRootDir, v)
	oldname, err := idx.storage.ResolveSymlink(context.Background(), searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, IndexBy: idx.indexBy, Value: v}
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
	if err := idx.storage.CreateSymlink(context.Background(), id, newName); err != nil {
		if os.IsExist(err) {
			return "", &idxerrs.AlreadyExistsErr{TypeName: idx.typeName, IndexBy: idx.indexBy, Value: v}
		}

		return "", err
	}

	return newName, nil
}

// Remove a value v from an index.
func (idx *Autoincrement) Remove(_ string, v string) error {
	if v == "" {
		return nil
	}
	searchPath := path.Join(idx.indexRootDir, v)
	_, err := idx.storage.ResolveSymlink(context.Background(), searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = &idxerrs.NotFoundErr{TypeName: idx.typeName, IndexBy: idx.indexBy, Value: v}
		}

		return err
	}

	deletePath := path.Join("/", idx.indexRootDir, v)
	return idx.storage.Delete(context.Background(), deletePath)
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
	paths, err := idx.storage.ReadDir(context.Background(), idx.indexRootDir)
	if err != nil {
		return nil, err
	}

	searchPath := idx.indexRootDir
	matches := make([]string, 0)
	for _, p := range paths {
		if found, err := filepath.Match(pattern, path.Base(p)); found {
			if err != nil {
				return nil, err
			}

			oldPath, err := idx.storage.ResolveSymlink(context.Background(), path.Join(searchPath, path.Base(p)))
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
func (idx *Autoincrement) IndexBy() option.IndexBy {
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
	paths, err := idx.storage.ReadDir(context.Background(), idx.indexRootDir)

	if err != nil {
		return -1, err
	}

	if len(paths) == 0 {
		return int(idx.bound.Lower), nil
	}

	sort.Slice(paths, func(i, j int) bool {
		a, _ := strconv.Atoi(path.Base(paths[i]))
		b, _ := strconv.Atoi(path.Base(paths[j]))
		return a < b
	})

	latest, err := strconv.Atoi(path.Base(paths[len(paths)-1])) // would returning a string be a better interface?
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
	return idx.storage.Delete(context.Background(), idx.indexRootDir)
}
