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

package sharedconf

import (
	"sync"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
)

var sharedConf *config.Shared = &config.Shared{}
var once sync.Once

func Init(c *config.Shared) {
	once.Do(func() {
		sharedConf = c
	})
}

// GetJWTSecret returns the package level configured jwt secret if not overwritten.
func GetJWTSecret(val string) string {
	if val == "" {
		return sharedConf.JWTSecret
	}
	return val
}

// GetGatewaySVC returns the package level configured gateway service if not overwritten.
func GetGatewaySVC(val string) string {
	if val == "" {
		return sharedConf.GatewaySVC
	}
	return val
}

// GetDataGateway returns the package level data gateway endpoint if not overwritten.
func GetDataGateway(val string) string {
	if val == "" {
		return sharedConf.DataGateway
	}
	return val
}

// SkipUserGroupsInToken returns whether to skip encoding user groups in the access tokens.
func SkipUserGroupsInToken() bool {
	return sharedConf.SkipUserGroupsInToken
}

// GetBlockedUsers returns a list of blocked users.
func GetBlockedUsers() []string {
	return sharedConf.BlockedUsers
}
