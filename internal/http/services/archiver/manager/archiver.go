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

package manager

import (
	"archive/tar"
	"archive/zip"
	"context"
	"io"
	"path"
	"path/filepath"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
)

// Config is the config for the Archiver.
type Config struct {
	MaxNumFiles int64
	MaxSize     int64
}

// Archiver is the struct able to create an archive.
type Archiver struct {
	files      []string
	dir        string
	walker     walker.Walker
	downloader downloader.Downloader
	config     Config
}

// NewArchiver creates a new archiver able to create an archive containing the files in the list.
func NewArchiver(files []string, w walker.Walker, d downloader.Downloader, config Config) (*Archiver, error) {
	if len(files) == 0 {
		return nil, ErrEmptyList{}
	}

	dir := getDeepestCommonDir(files)
	if pathIn(files, dir) {
		dir = filepath.Dir(dir)
	}

	arc := &Archiver{
		dir:        dir,
		files:      files,
		walker:     w,
		downloader: d,
		config:     config,
	}
	return arc, nil
}

// pathIn verifies that the path `f`is in the `files`list.
func pathIn(files []string, f string) bool {
	f = filepath.Clean(f)
	for _, file := range files {
		if filepath.Clean(file) == f {
			return true
		}
	}
	return false
}

func getDeepestCommonDir(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// find the maximum common substring from left
	res := path.Clean(files[0]) + "/"

	for _, file := range files[1:] {
		file = path.Clean(file) + "/"

		if len(file) < len(res) {
			res, file = file, res
		}

		for i := 0; i < len(res); i++ {
			if res[i] != file[i] {
				res = res[:i]
			}
		}
	}

	// the common substring could be between two / - inside a file name
	for i := len(res) - 1; i >= 0; i-- {
		if res[i] == '/' {
			res = res[:i+1]
			break
		}
	}
	return filepath.Clean(res)
}

// CreateTar creates a tar and write it into the dst Writer.
func (a *Archiver) CreateTar(ctx context.Context, dst io.Writer) error {
	w := tar.NewWriter(dst)

	var filesCount, sizeFiles int64

	for _, root := range a.files {
		err := a.walker.Walk(ctx, root, func(path string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				return err
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			filesCount++
			if filesCount > a.config.MaxNumFiles {
				return ErrMaxFileCount{}
			}

			if !isDir {
				// only add the size if the resource is not a directory
				// as its size could be resursive-computed, and we would
				// count the files not only once
				sizeFiles += int64(info.Size)
				if sizeFiles > a.config.MaxSize {
					return ErrMaxSize{}
				}
			}

			// TODO (gdelmont): remove duplicates if the resources requested overlaps
			fileName, err := filepath.Rel(a.dir, path)

			if err != nil {
				return err
			}

			header := tar.Header{
				Name:    fileName,
				ModTime: time.Unix(int64(info.Mtime.Seconds), 0),
			}

			if isDir {
				// the resource is a folder
				header.Mode = 0755
				header.Typeflag = tar.TypeDir
			} else {
				header.Mode = 0644
				header.Typeflag = tar.TypeReg
				header.Size = int64(info.Size)
			}

			err = w.WriteHeader(&header)
			if err != nil {
				return err
			}

			if !isDir {
				r, err := a.downloader.Download(ctx, path, "")
				if err != nil {
					return err
				}
				if _, err := io.Copy(w, r); err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}
	}
	return w.Close()
}

// CreateZip creates a zip and write it into the dst Writer.
func (a *Archiver) CreateZip(ctx context.Context, dst io.Writer) error {
	w := zip.NewWriter(dst)

	var filesCount, sizeFiles int64

	for _, root := range a.files {
		err := a.walker.Walk(ctx, root, func(path string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				return err
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			filesCount++
			if filesCount > a.config.MaxNumFiles {
				return ErrMaxFileCount{}
			}

			if !isDir {
				// only add the size if the resource is not a directory
				// as its size could be resursive-computed, and we would
				// count the files not only once
				sizeFiles += int64(info.Size)
				if sizeFiles > a.config.MaxSize {
					return ErrMaxSize{}
				}
			}

			// TODO (gdelmont): remove duplicates if the resources requested overlaps
			fileName, err := filepath.Rel(a.dir, path)
			if err != nil {
				return err
			}

			if fileName == "" {
				return nil
			}

			header := zip.FileHeader{
				Name:     fileName,
				Modified: time.Unix(int64(info.Mtime.Seconds), 0),
			}

			if isDir {
				header.Name += "/"
			} else {
				header.UncompressedSize64 = info.Size
			}

			dst, err := w.CreateHeader(&header)
			if err != nil {
				return err
			}

			if !isDir {
				r, err := a.downloader.Download(ctx, path, "")
				if err != nil {
					return err
				}
				if _, err := io.Copy(dst, r); err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}
	}
	return w.Close()
}
