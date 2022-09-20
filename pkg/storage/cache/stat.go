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

package cache

import (
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// StatCache can invalidate all stat related cache entries
type StatCache struct {
	Cache
}

func NewStatCache(store string, nodes []string, ttl time.Duration) StatCache {
	c := StatCache{}
	c.s = getStore(store, nodes, ttl)
	c.ttl = ttl
	return c
}

// RemoveStat removes a reference from the stat cache
func (c StatCache) RemoveStat(user *userpb.User, res *provider.ResourceId) {
	uid := "uid:" + user.Id.OpaqueId
	sid := ""
	oid := ""
	if res != nil {
		sid = "sid:" + res.SpaceId
		oid = "oid:" + res.OpaqueId
	}

	keys, _ := c.s.List()
	// FIMXE handle error
	for _, key := range keys {
		if strings.Contains(key, uid) {
			_ = c.s.Delete(key)
			continue
		}

		if sid != "" && strings.Contains(key, sid) {
			_ = c.s.Delete(key)
			continue
		}

		if oid != "" && strings.Contains(key, oid) {
			_ = c.s.Delete(key)
			continue
		}
	}
}
