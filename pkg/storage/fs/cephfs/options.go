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

package cephfs

// Options for the cephfs module
type Options struct {
	Root           string `mapstructure:"root"`
	ShareFolder    string `mapstructure:"share_folder"`
	Uploads        string `mapstructure:"uploads"`
	Shadow         string `mapstructure:"shadow"`
	References     string `mapstructure:"references"`
	GatewaySvc     string `mapstructure:"gatewaysvc"`
	DirMode        uint32 `mapstructure:"dirmode"`
	DisableHome    bool   `mapstructure:"disable_home"`
	UserQuotaBytes uint64 `mapstructure:"user_quota_bytes"`
}
