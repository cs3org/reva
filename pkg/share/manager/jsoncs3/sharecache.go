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

package jsoncs3

import (
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storagespace"
)

type ShareCache struct {
	UserShares map[string]*UserShareCache
}

type UserShareCache struct {
	Mtime      time.Time
	UserShares map[string]*SpaceShareIDs
}

type SpaceShareIDs struct {
	Mtime time.Time
	IDs   map[string]struct{}
}

func NewShareCache() ShareCache {
	return ShareCache{
		UserShares: map[string]*UserShareCache{},
	}
}

func (c *ShareCache) Has(userid string) bool {
	return c.UserShares[userid] != nil
}
func (c *ShareCache) GetShareCache(userid string) *UserShareCache {
	return c.UserShares[userid]
}

func (c *ShareCache) SetShareCache(userid string, shareCache *UserShareCache) {
	c.UserShares[userid] = shareCache
}

func (c *ShareCache) Add(userid, shareID string) error {
	storageid, spaceid, _, err := storagespace.SplitID(shareID)
	if err != nil {
		return err
	}
	ssid := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: storageid,
		SpaceId:   spaceid,
	})

	now := time.Now()
	if c.UserShares[userid] == nil {
		c.UserShares[userid] = &UserShareCache{
			UserShares: map[string]*SpaceShareIDs{},
		}
	}
	if c.UserShares[userid].UserShares[ssid] == nil {
		c.UserShares[userid].UserShares[ssid] = &SpaceShareIDs{
			IDs: map[string]struct{}{},
		}
	}
	// add share id
	c.UserShares[userid].Mtime = now
	c.UserShares[userid].UserShares[ssid].Mtime = now
	c.UserShares[userid].UserShares[ssid].IDs[shareID] = struct{}{}
	return nil
}

func (c *ShareCache) Remove(userid, shareID string) error {
	storageid, spaceid, _, err := storagespace.SplitID(shareID)
	if err != nil {
		return err
	}
	ssid := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: storageid,
		SpaceId:   spaceid,
	})

	if c.UserShares[userid] != nil {
		if c.UserShares[userid].UserShares[ssid] != nil {
			// remove share id
			now := time.Now()
			c.UserShares[userid].Mtime = now
			c.UserShares[userid].UserShares[ssid].Mtime = now
			delete(c.UserShares[userid].UserShares[ssid].IDs, shareID)
		}
	}
	return nil
}

func (c *ShareCache) List(userid string) map[string]SpaceShareIDs {
	r := map[string]SpaceShareIDs{}
	if c.UserShares[userid] == nil {
		return r
	}

	for ssid, cached := range c.UserShares[userid].UserShares {
		r[ssid] = SpaceShareIDs{
			Mtime: cached.Mtime,
			IDs:   cached.IDs,
		}
	}
	return r
}
