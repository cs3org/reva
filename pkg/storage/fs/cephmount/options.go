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

package cephmount

// Options for the cephmount module
type Options struct {
	UploadFolder   string `mapstructure:"uploads"`
	DirPerms       uint32 `mapstructure:"dir_perms"`
	FilePerms      uint32 `mapstructure:"file_perms"`
	UserQuotaBytes uint64 `mapstructure:"user_quota_bytes"`

	// Nobody user/group for fallback operations (instead of root)
	NobodyUID int `mapstructure:"nobody_uid"`
	NobodyGID int `mapstructure:"nobody_gid"`

	// External (lightweight, federated, guest) accounts have no uid of their own,
	// so operations on their behalf run as this local service account instead.
	// What they are allowed to do is decided by reva from the stored grants, not
	// by the permissions of this account.
	ExternalAccountsUserName string `mapstructure:"external_accounts_user_name"`
	ExternalAccountsUserUID  int    `mapstructure:"external_accounts_user_uid"`
	ExternalAccountsUserGID  int    `mapstructure:"external_accounts_user_gid"`

	// SpaceDepth defines how many levels below the chroot the space (project)
	// roots sit: with roots at /winspaces/c/<project>, served by a provider
	// mounted at /winspaces, it is 2. The service account is granted rwx on the
	// space root when something inside it is shared with an external account.
	//
	// It is injected by the storage provider, which rebases its own space_depth
	// on the paths it hands to the driver: its mount path is trimmed off first,
	// so the number here is smaller than the one in its configuration. It should
	// not be set in the driver configuration.
	SpaceDepth int `mapstructure:"space_depth"`

	// Simplified Ceph configuration - just paste the fstab entry
	FstabEntry string `mapstructure:"fstabentry"` // Complete fstab line for Ceph mount
	RootDir    string `mapstructure:"root_dir"`   // Root directory for the Ceph mount
	FsName     string `mapstructure:"fs_name"`    // Ceph filesystem name (e.g., "cephfs")

	// Testing-only option - allows running without Ceph configuration for local filesystem tests
	TestingAllowLocalMode bool `mapstructure:"testing_allow_local_mode"` // Bypass fstab parsing requirement for tests only

	HiddenDirs map[string]bool
}

func (c *Options) ApplyDefaults() {
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

	// Same service account the eos driver uses for external accounts.
	if c.ExternalAccountsUserName == "" || c.ExternalAccountsUserUID == 0 || c.ExternalAccountsUserGID == 0 {
		c.ExternalAccountsUserName = "cboxexternal"
		c.ExternalAccountsUserUID = 193339
		c.ExternalAccountsUserGID = 2766
	}

	// SpaceDepth is deliberately left alone: it comes from the storage provider,
	// and guessing a layout here would place the service account ACL on the wrong
	// directory. New warns when it is missing.

	// No Ceph defaults needed - everything is extracted from the fstab entry
	// Chroot directory defaults will be set from fstab parsing (local mount point)

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
