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

package nceph

// Options for the nceph module
type Options struct {
	Root           string `mapstructure:"root"`
	UploadFolder   string `mapstructure:"uploads"`
	DirPerms       uint32 `mapstructure:"dir_perms"`
	FilePerms      uint32 `mapstructure:"file_perms"`
	UserQuotaBytes uint64 `mapstructure:"user_quota_bytes"`

	// Nobody user/group for fallback operations (instead of root)
	NobodyUID int `mapstructure:"nobody_uid"`
	NobodyGID int `mapstructure:"nobody_gid"`

	// Ceph configuration for GetPathByID operations
	CephConfig   string `mapstructure:"ceph_config"`    // Path to /etc/ceph/ceph.conf
	CephClientID string `mapstructure:"ceph_client_id"` // Ceph client ID (from keyring)
	CephKeyring  string `mapstructure:"ceph_keyring"`   // Path to keyring file
	CephRoot     string `mapstructure:"ceph_root"`      // Application-level: mount root for MountWithRoot() (NOT a Ceph config directive)

	HiddenDirs map[string]bool
}

func (c *Options) ApplyDefaults() {
	if c.Root == "" {
		c.Root = "/mnt/cephfs/"
	} else {
		c.Root = addLeadingSlash(c.Root) //force absolute path in case leading "/" is omitted
	}

	if c.UploadFolder == "" {
		c.UploadFolder = ".uploads"
	}

	// Nobody user/group defaults (commonly 65534 on Linux systems)
	if c.NobodyUID == 0 {
		c.NobodyUID = 65534
	}

	if c.NobodyGID == 0 {
		c.NobodyGID = 65534
	}

	// Ceph defaults for GetPathByID operations
	if c.CephConfig == "" {
		c.CephConfig = "/etc/ceph/ceph.conf"
	} else {
		c.CephConfig = addLeadingSlash(c.CephConfig)
	}

	if c.CephClientID == "" {
		c.CephClientID = "admin"
	}

	if c.CephKeyring == "" {
		c.CephKeyring = "/etc/ceph/keyring"
	} else {
		c.CephKeyring = addLeadingSlash(c.CephKeyring)
	}

	if c.CephRoot == "" {
		c.CephRoot = "/cephfs"
	} else {
		c.CephRoot = addLeadingSlash(c.CephRoot)
	}

	c.HiddenDirs = map[string]bool{
		".":                                true,
		"..":                               true,
		removeLeadingSlash(c.UploadFolder): true,
	}

	if c.DirPerms == 0 {
		c.DirPerms = dirPermDefault
	}

	if c.FilePerms == 0 {
		c.FilePerms = filePermDefault
	}

	if c.UserQuotaBytes == 0 {
		c.UserQuotaBytes = 50000000000 // 50GB default
	}
}

var dirPermDefault = uint32(0755)
var filePermDefault = uint32(0644)

func addLeadingSlash(path string) string {
	if path[0] != '/' {
		return "/" + path
	}
	return path
}

func removeLeadingSlash(path string) string {
	if path[0] == '/' {
		return path[1:]
	}
	return path
}
