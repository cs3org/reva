// Copyright 2018-2021 CERN
// Copyright 2025 OpenCloud GmbH <mail@opencloud.eu>
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

package blobstore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"

	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	"github.com/pkg/errors"
)

// Blobstore provides an interface to an filesystem based blobstore
type Blobstore struct {
	root string
}

// New returns a new Blobstore
func New(root string) (*Blobstore, error) {
	return &Blobstore{
		root: root,
	}, nil
}

// Upload stores some data in the blobstore under the given key
func (bs *Blobstore) Upload(node *node.Node, source, copyTarget string) error {
	path := node.InternalPath()

	// preserve the mtime of the file
	fi, _ := os.Stat(path)

	file, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: posix blobstore: Can not open source file to upload")
	}
	defer file.Close()

	f, err := os.OpenFile(node.InternalPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return errors.Wrapf(err, "could not open blob '%s' for writing", node.InternalPath())
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	_, err = w.ReadFrom(file)
	if err != nil {
		return errors.Wrapf(err, "could not write blob '%s'", node.InternalPath())
	}
	err = w.Flush()
	if err != nil {
		return err
	}
	err = os.Chtimes(path, fi.ModTime(), fi.ModTime())
	if err != nil {
		return err
	}

	if copyTarget != "" {
		// also "upload" the file to a local path, e.g. for keeping the "current" version of the file
		err := os.MkdirAll(filepath.Dir(copyTarget), 0700)
		if err != nil {
			return err
		}

		_, err = file.Seek(0, 0)
		if err != nil {
			return err
		}

		copyFile, err := os.OpenFile(copyTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return errors.Wrapf(err, "could not open copy target '%s' for writing", copyTarget)
		}
		defer copyFile.Close()

		_, err = copyFile.ReadFrom(file)
		if err != nil {
			return errors.Wrapf(err, "could not write blob copy of '%s' to '%s'", node.InternalPath(), copyTarget)
		}
	}

	return nil
}

// Download retrieves a blob from the blobstore for reading
func (bs *Blobstore) Download(node *node.Node) (io.ReadCloser, error) {
	file, err := os.Open(node.InternalPath())
	if err != nil {
		return nil, errors.Wrapf(err, "could not read blob '%s'", node.InternalPath())
	}
	return file, nil
}

// Delete deletes a blob from the blobstore
func (bs *Blobstore) Delete(node *node.Node) error {
	return nil
}
