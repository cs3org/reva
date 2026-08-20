// Copyright 2018-2024 CERN
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
	"context"
	"io"

	"github.com/cs3org/reva/v3/pkg/bundler"
	"github.com/cs3org/reva/v3/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/v3/pkg/storage/utils/walker"
)

// Config is the config for the Archiver.
type Config struct {
	MaxNumFiles int64
	MaxSize     int64
}

// Archiver is the struct able to create an archive.
type Archiver struct {
	files   []string
	bundler *bundler.Bundler
	config  Config
}

// NewArchiver creates a new archiver able to create an archive containing the files in the list.
func NewArchiver(files []string, w walker.Walker, d downloader.Downloader, config Config) (*Archiver, error) {
	if len(files) == 0 {
		return nil, bundler.ErrEmptyList{}
	}

	arc := &Archiver{
		files:   files,
		bundler: bundler.New(w, d),
		config:  config,
	}
	return arc, nil
}

// opts returns the bundler options tailored for the archiver
func (a *Archiver) opts(f bundler.Format) bundler.Options {
	return bundler.Options{
		Roots:        a.files,
		Format:       f,
		MaxNumFiles:  a.config.MaxNumFiles,
		MaxTotalSize: a.config.MaxSize,
	}
}

// CreateTar creates a tar and write it into the dst Writer.
func (a *Archiver) CreateTar(ctx context.Context, dst io.Writer) error {
	return a.bundler.Create(ctx, a.opts(bundler.FormatTar), bundler.SinglePart(dst))
}

// CreateZip creates a zip and write it into the dst Writer.
func (a *Archiver) CreateZip(ctx context.Context, dst io.Writer) error {
	return a.bundler.Create(ctx, a.opts(bundler.FormatZip), bundler.SinglePart(dst))
}
