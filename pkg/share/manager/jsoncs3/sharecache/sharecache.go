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

package sharecache

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
)

type ShareCache struct {
	UserShares map[string]*UserShareCache

	storage metadata.Storage
}

type UserShareCache struct {
	Mtime      time.Time
	UserShares map[string]*SpaceShareIDs
}

type SpaceShareIDs struct {
	Mtime time.Time
	IDs   map[string]struct{}
}

func New(s metadata.Storage) ShareCache {
	return ShareCache{
		UserShares: map[string]*UserShareCache{},
		storage:    s,
	}
}

func (c *ShareCache) Has(userid string) bool {
	return c.UserShares[userid] != nil
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

func (c *ShareCache) Sync(ctx context.Context, userid string) error {
	var mtime time.Time
	//  - do we have a cached list of created shares for the user in memory?
	if usc := c.UserShares[userid]; usc != nil {
		mtime = usc.Mtime
		//    - y: set If-Modified-Since header to only download if it changed
	} else {
		mtime = time.Time{} // Set zero time so that data from storage always takes precedence
	}

	userCreatedPath := userCreatedPath(userid)
	info, err := c.storage.Stat(ctx, userCreatedPath)
	if err != nil {
		return err
	}
	// check mtime of /users/{userid}/created.json
	if utils.TSToTime(info.Mtime).After(mtime) {
		//  - update cached list of created shares for the user in memory if changed
		createdBlob, err := c.storage.SimpleDownload(ctx, userCreatedPath)
		if err != nil {
			return err
		}
		newShareCache := &UserShareCache{}
		err = json.Unmarshal(createdBlob, newShareCache)
		if err != nil {
			return err
		}
		c.UserShares[userid] = newShareCache
	}
	return nil
}

func (c *ShareCache) Persist(ctx context.Context, userid string) error {
	createdBytes, err := json.Marshal(c.UserShares[userid])
	if err != nil {
		return err
	}
	// FIXME needs stat & upload if match combo to prevent lost update in redundant deployments
	if err := c.storage.SimpleUpload(ctx, userCreatedPath(userid), createdBytes); err != nil {
		return err
	}
	return nil
}

func userCreatedPath(userid string) string {
	userCreatedPath := filepath.Join("/users", userid, "created.json")
	return userCreatedPath
}
