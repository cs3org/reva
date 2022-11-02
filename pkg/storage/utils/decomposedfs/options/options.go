// Copyright 2018-2021 CERN
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

package options

import (
	"path/filepath"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	// ocis fs works on top of a dir of uuid nodes
	Root string `mapstructure:"root"`

	// UserLayout describes the relative path from the storage's root node to the users home node.
	UserLayout string `mapstructure:"user_layout"`

	// TODO NodeLayout option to save nodes as eg. nodes/1d/d8/1dd84abf-9466-4e14-bb86-02fc4ea3abcf
	ShareFolder string `mapstructure:"share_folder"`

	// propagate mtime changes as tmtime (tree modification time) to the parent directory when user.ocis.propagation=1 is set on a node
	TreeTimeAccounting bool `mapstructure:"treetime_accounting"`

	// propagate size changes as treesize
	TreeSizeAccounting bool `mapstructure:"treesize_accounting"`

	// permissions service to use when checking permissions
	PermissionsSVC string `mapstructure:"permissionssvc"`

	PersonalSpaceAliasTemplate string `mapstructure:"personalspacealias_template"`
	GeneralSpaceAliasTemplate  string `mapstructure:"generalspacealias_template"`

	AsyncFileUploads bool `mapstructure:"asyncfileuploads"`

	Events EventOptions `mapstructure:"events"`

	Tokens TokenOptions `mapstructure:"tokens"`

	StatCache CacheOptions `mapstructure:"statcache"`

	MaxAcquireLockCycles int `mapstructure:"max_acquire_lock_cycles"`
}

// EventOptions are the configurable options for events
type EventOptions struct {
	NatsAddress          string `mapstructure:"natsaddress"`
	NatsClusterID        string `mapstructure:"natsclusterid"`
	TLSInsecure          bool   `mapstructure:"tlsinsecure"`
	TLSRootCACertificate string `mapstructure:"tlsrootcacertificate"`
	NumConsumers         int    `mapstructure:"numconsumers"`
}

// TokenOptions are the configurable option for tokens
type TokenOptions struct {
	DownloadEndpoint     string `mapstructure:"download_endpoint"`
	DataGatewayEndpoint  string `mapstructure:"datagateway_endpoint"`
	TransferSharedSecret string `mapstructure:"transfer_shared_secret"`
	TransferExpires      int64  `mapstructure:"transfer_expires"`
}

// CacheOptions contains options of configuring a cache
type CacheOptions struct {
	CacheStore    string   `mapstructure:"cache_store"`
	CacheNodes    []string `mapstructure:"cache_nodes"`
	CacheDatabase string   `mapstructure:"cache_database"`
}

// New returns a new Options instance for the given configuration
func New(m map[string]interface{}) (*Options, error) {
	o := &Options{}
	if err := mapstructure.Decode(m, o); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	if o.UserLayout == "" {
		o.UserLayout = "{{.Id.OpaqueId}}"
	}
	// ensure user layout has no starting or trailing /
	o.UserLayout = strings.Trim(o.UserLayout, "/")

	if o.ShareFolder == "" {
		o.ShareFolder = "/Shares"
	}
	// ensure share folder always starts with slash
	o.ShareFolder = filepath.Join("/", o.ShareFolder)

	// c.DataDirectory should never end in / unless it is the root
	o.Root = filepath.Clean(o.Root)

	if o.PersonalSpaceAliasTemplate == "" {
		o.PersonalSpaceAliasTemplate = "{{.SpaceType}}/{{.User.Username}}"
	}

	if o.GeneralSpaceAliasTemplate == "" {
		o.GeneralSpaceAliasTemplate = "{{.SpaceType}}/{{.SpaceName | replace \" \" \"-\" | lower}}"
	}

	return o, nil
}
