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

//go:build ceph
// +build ceph

package cephfs

import (
	"path/filepath"

	"github.com/cs3org/reva/pkg/sharedconf"
)

// Options for the cephfs module
type Options struct {
	ClientID     string `mapstructure:"client_id"`
	Config       string `mapstructure:"config"`
	GatewaySvc   string `mapstructure:"gatewaysvc"`
	IndexPool    string `mapstructure:"index_pool"`
	Keyring      string `mapstructure:"keyring"`
	Root         string `mapstructure:"root"`
	ShadowFolder string `mapstructure:"shadow_folder"`
	ShareFolder  string `mapstructure:"share_folder"`
	UploadFolder string `mapstructure:"uploads"`
	UserLayout   string `mapstructure:"user_layout"`

	DisableHome    bool   `mapstructure:"disable_home"`
	DirPerms       uint32 `mapstructure:"dir_perms"`
	FilePerms      uint32 `mapstructure:"file_perms"`
	UserQuotaBytes uint64 `mapstructure:"user_quota_bytes"`
	HiddenDirs     map[string]bool
}

func (c *Options) fillDefaults() {
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
		c.Root = "/home"
	} else {
		c.Root = addLeadingSlash(c.Root)
	}

	if c.ShadowFolder == "" {
		c.ShadowFolder = "/.reva_hidden"
	} else {
		c.ShadowFolder = addLeadingSlash(c.ShadowFolder)
	}

	if c.ShareFolder == "" {
		c.ShareFolder = "/Shares"
	} else {
		c.ShareFolder = addLeadingSlash(c.ShareFolder)
	}

	if c.UploadFolder == "" {
		c.UploadFolder = ".uploads"
	}
	c.UploadFolder = filepath.Join(c.ShadowFolder, c.UploadFolder)

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}

	c.HiddenDirs = map[string]bool{
		".":                                true,
		"..":                               true,
		removeLeadingSlash(c.ShadowFolder): true,
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
}
