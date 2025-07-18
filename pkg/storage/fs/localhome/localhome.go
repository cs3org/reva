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

package localhome

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/cs3org/reva/v3/pkg/storage/utils/localfs"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("localhome", New)
}

type config struct {
	Root        string `docs:"/var/tmp/reva/;Path of root directory for user storage." mapstructure:"root"`
	ShareFolder string `docs:"/MyShares;Path for storing share references."            mapstructure:"share_folder"`
	UserLayout  string `docs:"{{.Username}};Template for user home directories"        mapstructure:"user_layout"`
}

func (c *config) ApplyDefaults() {
	if c.Root == "" {
		c.Root = "/var/tmp/reva"
	}
	if c.ShareFolder == "" {
		c.ShareFolder = "/MyShares"
	}
	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talks to
// a local filesystem with user homes.
func New(ctx context.Context, m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	conf := localfs.Config{
		Root:        c.Root,
		ShareFolder: c.ShareFolder,
		UserLayout:  c.UserLayout,
	}
	return localfs.NewLocalFS(&conf)
}
