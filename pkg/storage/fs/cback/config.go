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

package cback

import provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

// Config for the cback driver
type Config struct {
	Token             string `mapstructure:"token"`
	APIURL            string `mapstructure:"api_url"`
	Insecure          bool   `mapstructure:"insecure"`
	Timeout           int    `mapstructure:"timeout"`
	Size              int    `mapstructure:"size"`
	Expiration        int    `mapstructure:"expiration"`
	TemplateToStorage string `mapstructure:"template_to_storage"`
	TemplateToCback   string `mapstructure:"template_to_cback"`
}

func (c *Config) init() {
	if c.Size == 0 {
		c.Size = 1_000_000
	}

	if c.Expiration == 0 {
		c.Expiration = 300
	}

	if c.TemplateToCback == "" {
		c.TemplateToCback = "{{ . }}"
	}

	if c.TemplateToStorage == "" {
		c.TemplateToStorage = "{{ . }}"
	}
}

var permDir = &provider.ResourcePermissions{
	AddGrant:             false,
	CreateContainer:      false,
	Delete:               false,
	GetPath:              true,
	GetQuota:             true,
	InitiateFileDownload: true,
	InitiateFileUpload:   false,
	ListGrants:           true,
	ListContainer:        true,
	ListFileVersions:     true,
	ListRecycle:          false,
	Move:                 false,
	RemoveGrant:          false,
	PurgeRecycle:         false,
	RestoreFileVersion:   false,
	RestoreRecycleItem:   false,
	Stat:                 true,
	UpdateGrant:          false,
	DenyGrant:            false,
}

var permFile = &provider.ResourcePermissions{
	AddGrant:             false,
	CreateContainer:      false,
	Delete:               false,
	GetPath:              true,
	GetQuota:             true,
	InitiateFileDownload: true,
	InitiateFileUpload:   false,
	ListGrants:           true,
	ListContainer:        false,
	ListFileVersions:     true,
	ListRecycle:          false,
	Move:                 false,
	RemoveGrant:          false,
	PurgeRecycle:         false,
	RestoreFileVersion:   false,
	RestoreRecycleItem:   false,
	Stat:                 true,
	UpdateGrant:          false,
	DenyGrant:            false,
}
