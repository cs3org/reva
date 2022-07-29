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
	"errors"
	"time"

	"github.com/bluele/gcache"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storagespace"
)

type shareCache struct {
	userShares map[string]*userShareCache
}

type userShareCache struct {
	mtime      time.Time
	userShares gcache.Cache
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
func (c *shareCache) Get(userid string) *userShareCache {
	return c.userShares[userid]
}

func (c *shareCache) Add(userid, shareID string) error {
	now := time.Now()
	if _, ok := c.userShares[userid]; !ok {
		c.userShares[userid] = &userShareCache{
			userShares: gcache.New(-1).Simple().Build(),
		}
	}
	c.userShares[userid].mtime = now
	storageid, spaceid, _, err := storagespace.SplitID(shareID)
	if err != nil {
		return err
	}
	spaceId := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: storageid,
		SpaceId:   spaceid,
	})
	v, err := c.userShares[userid].userShares.Get(spaceId)
	switch {
	case err == gcache.KeyNotFoundError:
		// create new entry
		return c.userShares[userid].userShares.Set(spaceId, &spaceShareIDs{
			mtime: now,
			IDs:   map[string]struct{}{shareID: {}},
		})
	case err != nil:
		return err
	}
	// update list
	spaceShareIDs, ok := v.(*spaceShareIDs)
	if !ok {
		return errors.New("invalid type")
	}
	spaceShareIDs.IDs[shareID] = struct{}{}
	return nil
}

func (c *shareCache) List(userid string) map[string]spaceShareIDs {
	r := make(map[string]spaceShareIDs)
	for k, v := range c.userShares[userid].userShares.GetALL(false) {

		var ssid string
		var cached *spaceShareIDs
		var ok bool
		if ssid, ok = k.(string); !ok {
			continue
		}
		if cached, ok = v.(*spaceShareIDs); !ok {
			continue
		}
		r[ssid] = spaceShareIDs{
			mtime: cached.mtime,
			IDs:   cached.IDs,
		}
	}
	return r
}
