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

package local

import (
	"context"

	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/localfs"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	registry.Register("local", New)
}

type config struct {
	Root        string `docs:"/var/tmp/reva/;Path of root directory for user storage." mapstructure:"root"`
	ShareFolder string `docs:"/MyShares;Path for storing share references."            mapstructure:"share_folder"`
}

func (c *config) ApplyDefaults() {
	if c.Root == "" {
		c.Root = "/var/tmp/reva"
	}
	if c.ShareFolder == "" {
		c.ShareFolder = "/MyShares"
	}
}

// New returns an implementation to of the storage.FS interface that talks to
// a local filesystem with user homes disabled.
func New(ctx context.Context, m map[string]interface{}) (storage.FS, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	conf := localfs.Config{
		Root:        c.Root,
		ShareFolder: c.ShareFolder,
		DisableHome: true,
	}
	return localfs.NewLocalFS(&conf)
}
