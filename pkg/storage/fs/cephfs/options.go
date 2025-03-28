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

//go:build ceph
// +build ceph

package cephfs

import (
	"github.com/cs3org/reva/pkg/sharedconf"
)

// Options for the cephfs module
type Options struct {
	ClientID       string `mapstructure:"client_id"`
	Config         string `mapstructure:"config"`
	GatewaySvc     string `mapstructure:"gatewaysvc"`
	IndexPool      string `mapstructure:"index_pool"`
	Keyring        string `mapstructure:"keyring"`
	Root           string `mapstructure:"root"`
	UploadFolder   string `mapstructure:"uploads"`
	UserLayout     string `mapstructure:"user_layout"`
	DirPerms       uint32 `mapstructure:"dir_perms"`
	FilePerms      uint32 `mapstructure:"file_perms"`
	UserQuotaBytes uint64 `mapstructure:"user_quota_bytes"`
	// Path of the recycle bin. If empty, recycling is disabled.
	RecyclePath string `mapstructure:"recycle_path"`
	// Depth of the Recycle bin location, that is after how many path components
	// the recycle path is located: this allows supporting recycles such as
	// /top-level/s/space/.recycle with a depth = 3. Defaults to 0.
	RecyclePathDepth int `mapstructure:"recycle_path_depth"`
	// Maximum entries count a ListRecycle call may return: if exceeded, ListRecycle
	// will return a BadRequest error
	MaxRecycleEntries int `mapstructure:"max_recycle_entries"`
	// Maximum time span in days a ListRecycle call may return: if exceeded, ListRecycle
	// will override the "to" date with "from" + this value
	MaxDaysInRecycleList int `mapstructure:"max_days_in_recycle_list"`
	HiddenDirs           map[string]bool
}

func (c *Options) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.IndexPool == "" {
		c.IndexPool = "path_index"
	}

	if c.Config == "" {
		c.Config = "/etc/ceph/ceph.conf"
	} else {
		c.Config = addLeadingSlash(c.Config) //force absolute path in case leading "/" is omitted
	}

	if c.ClientID == "" {
		c.ClientID = "admin"
	}

	if c.Keyring == "" {
		c.Keyring = "/etc/ceph/keyring"
	} else {
		c.Keyring = addLeadingSlash(c.Keyring)
	}

	if c.Root == "" {
		c.Root = "/cephfs"
	} else {
		c.Root = addLeadingSlash(c.Root)
	}

	if c.UploadFolder == "" {
		c.UploadFolder = ".uploads"
	}

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}

	c.HiddenDirs = map[string]bool{
		".":                                true,
		"..":                               true,
		removeLeadingSlash(c.UploadFolder): true,
	}
	if c.RecyclePath != "" {
		c.HiddenDirs[c.RecyclePath] = true
	}

	if c.DirPerms == 0 {
		c.DirPerms = dirPermDefault
	}

	if c.FilePerms == 0 {
		c.FilePerms = filePermDefault
	}

	if c.UserQuotaBytes == 0 {
		c.UserQuotaBytes = 50000000000
	}

	if c.MaxDaysInRecycleList == 0 {
		c.MaxDaysInRecycleList = 14
	}

	if c.MaxRecycleEntries == 0 {
		c.MaxRecycleEntries = 2000
	}
}
