// Copyright 2018-2023 CERN
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

package test

import (
	"bytes"
	"errors"
	"os"
	"path"
)

const (
	// TmpDirPattern is the pattern used for tmp folder creation.
	TmpDirPattern = "tmp-reva-"
)

// File struct represents a test file,
// with a certain content. Its name is defined in TestDir.
type File struct {
	Content string
}

// Dir struct represents a test dir, where each
// key is the resource (Dir or File) name.
type Dir map[string]interface{}

// CleanerFunc is a function to call after creating a TestDir.
type CleanerFunc func()

// TmpDir creates a dir in the system temp folder that has
// TmpDirPattern as prefix.
func TmpDir() (string, CleanerFunc, error) {
	name, err := os.MkdirTemp("", TmpDirPattern)
	if err != nil {
		return "", nil, err
	}

	c := func() {
		os.RemoveAll(name)
	}

	return name, c, nil
}

// NewTestDir creates the Dir structure in a local temporary folder.
func NewTestDir(src Dir) (tmpdir string, cleanup CleanerFunc, err error) {
	tmpdir, cleanup, err = TmpDir()
	if err != nil {
		return
	}
	err = newTestDirFileRecursive(tmpdir, src)
	return
}

// NewFile creates a new file given the path and the content.
func NewFile(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = file.WriteString(content)
	if err != nil {
		return err
	}
	return nil
}

func newTestDirFileRecursive(p string, res interface{}) error {
	switch r := res.(type) {
	case Dir:
		err := os.MkdirAll(p, 0755)
		if err != nil {
			return err
		}
		for name, content := range r {
			newPath := path.Join(p, name)
			err := newTestDirFileRecursive(newPath, content)
			if err != nil {
				return err
			}
		}
		return nil
	case File:
		return NewFile(p, r.Content)
	default:
		return errors.New("type not supported")
	}
}

// checks if the two files have the same content.
func fileEquals(file1, file2 string) bool {
	c1, _ := os.ReadFile(file1)
	c2, _ := os.ReadFile(file2)
	return bytes.Equal(c1, c2)
}

// DirEquals recursively verifies that the content of two dir is the same.
// Two files are equals if the name and the content is equal, while two folders
// are equal if the name is equal and the content is recursively equal.
func DirEquals(dir1, dir2 string) bool {
	l1, _ := os.ReadDir(dir1)
	l2, _ := os.ReadDir(dir2)

	if len(l1) != len(l2) {
		return false
	}

	// here l1 and l2 have the same length
	for i := range l1 {
		r1, r2 := l1[i], l2[i]
		if r1.Name() != r2.Name() {
			return false
		}

		path1, path2 := path.Join(dir1, r1.Name()), path.Join(dir2, r2.Name())

		switch {
		case r1.Type().IsDir() && r2.Type().IsDir(): // both are dirs
			if !DirEquals(path1, path2) {
				return false
			}
		case r1.Type().IsRegular() && r2.Type().IsRegular(): // both are files
			if !fileEquals(path1, path2) {
				return false
			}
		default: // different resource type
			return false
		}
	}
	return true
}
