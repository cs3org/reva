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

type shareCache struct {
	userShares map[string]*userShareCache
}

type userShareCache struct {
	mtime      time.Time
	userShares map[string]*spaceShareIDs
}

type spaceShareIDs struct {
	mtime time.Time
	IDs   map[string]struct{}
}

func NewShareCache() shareCache {
	return shareCache{
		userShares: map[string]*userShareCache{},
	}
}

func (c *shareCache) Has(userid string) bool {
	return c.userShares[userid] != nil
}
func (c *shareCache) GetShareCache(userid string) *userShareCache {
	return c.userShares[userid]
}

func (c *shareCache) SetShareCache(userid string, shareCache *userShareCache) {
	c.userShares[userid] = shareCache
}

func (c *shareCache) Add(userid, shareID string) error {
	storageid, spaceid, _, err := storagespace.SplitID(shareID)
	if err != nil {
		return err
	}
	ssid := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: storageid,
		SpaceId:   spaceid,
	})

	now := time.Now()
	if c.userShares[userid] == nil {
		c.userShares[userid] = &userShareCache{
			userShares: map[string]*spaceShareIDs{},
		}
	}
	if c.userShares[userid].userShares[ssid] == nil {
		c.userShares[userid].userShares[ssid] = &spaceShareIDs{
			IDs: map[string]struct{}{},
		}
	}
	// add share id
	c.userShares[userid].mtime = now
	c.userShares[userid].userShares[ssid].mtime = now
	c.userShares[userid].userShares[ssid].IDs[shareID] = struct{}{}
	return nil
}

func (c *shareCache) Remove(userid, shareID string) error {
	storageid, spaceid, _, err := storagespace.SplitID(shareID)
	if err != nil {
		return err
	}
	ssid := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: storageid,
		SpaceId:   spaceid,
	})

	if c.userShares[userid] != nil {
		if c.userShares[userid].userShares[ssid] != nil {
			// remove share id
			now := time.Now()
			c.userShares[userid].mtime = now
			c.userShares[userid].userShares[ssid].mtime = now
			delete(c.userShares[userid].userShares[ssid].IDs, shareID)
		}
	}
	return nil
}

func (c *shareCache) List(userid string) map[string]spaceShareIDs {
	r := map[string]spaceShareIDs{}
	if c.userShares[userid] == nil {
		return r
	}

	for ssid, cached := range c.userShares[userid].userShares {
		r[ssid] = spaceShareIDs{
			mtime: cached.mtime,
			IDs:   cached.IDs,
		}
	}
	return r
}
