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
	"time"

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

	Postprocessing PostprocessingOptions `mapstructure:"postprocessing"`
}

// InfectedFileOption are the options when finding an infected file
type InfectedFileOption string

var (
	// Delete deletes the infected file and cancels the upload
	Delete InfectedFileOption = "delete"
	// Keep will keep the infected file on disc but cancel the upload
	Keep InfectedFileOption = "keep"
	// Error will throw an error to the postprocessing but not cancel the upload
	Error InfectedFileOption = "error"
	// Ignore ignores the virus (except of marking it) and continues the upload regularly
	Ignore InfectedFileOption = "ignore"
)

// PostprocessingOptions defines the available options for postprocessing
type PostprocessingOptions struct {
	// do file assembling asynchronly
	AsyncFileUploads bool `mapstructure:"asyncfileuploads"`
	// scan files for viruses before assembling
	UploadVirusscan bool `mapstructure:"uploadvirusscan"`
	// the virusscanner to user
	VirusScanner string `mapstructure:"virusscanner"`
	// how to handle infected files
	InfectedFileHandling InfectedFileOption `mapstructure:"infectedfilehandling"`
	// for testing purposes, or if you want to annoy your users
	DelayProcessing time.Duration `mapstructure:"delayprocessing"`
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
