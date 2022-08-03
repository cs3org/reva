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

package providercache

import (
	"context"
	"encoding/json"
	"path"
	"path/filepath"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/v2/pkg/utils"
)

type Cache struct {
	Providers map[string]*Spaces

	storage metadata.Storage
}

type Spaces struct {
	Spaces map[string]*Shares
}

type Shares struct {
	Shares map[string]*collaboration.Share
	Mtime  time.Time
}

func New(s metadata.Storage) Cache {
	return Cache{
		Providers: map[string]*Spaces{},
		storage:   s,
	}
}

func (c *Cache) Add(storageID, spaceID, shareID string, share *collaboration.Share) {
	c.initializeIfNeeded(storageID, spaceID)
	c.Providers[storageID].Spaces[spaceID].Shares[shareID] = share
	c.Providers[storageID].Spaces[spaceID].Mtime = time.Now()
}

func (c *Cache) Remove(storageID, spaceID, shareID string) {
	if c.Providers[storageID] == nil ||
		c.Providers[storageID].Spaces[spaceID] == nil {
		return
	}
	delete(c.Providers[storageID].Spaces[spaceID].Shares, shareID)
	c.Providers[storageID].Spaces[spaceID].Mtime = time.Now()
}

func (c *Cache) Get(storageID, spaceID, shareID string) *collaboration.Share {
	if c.Providers[storageID] == nil ||
		c.Providers[storageID].Spaces[spaceID] == nil {
		return nil
	}
	return c.Providers[storageID].Spaces[spaceID].Shares[shareID]
}

func (c *Cache) ListSpace(storageID, spaceID string) *Shares {
	if c.Providers[storageID] == nil {
		return &Shares{}
	}
	return c.Providers[storageID].Spaces[spaceID]
}

func (c *Cache) Persist(ctx context.Context, storageID, spaceID string) error {
	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		return nil
	}

	createdBytes, err := json.Marshal(c.Providers[storageID].Spaces[spaceID])
	if err != nil {
		return err
	}
	jsonPath := spaceJSONPath(storageID, spaceID)
	// FIXME needs stat & upload if match combo to prevent lost update in redundant deployments
	if err := c.storage.MakeDirIfNotExist(ctx, path.Dir(jsonPath)); err != nil {
		return err
	}
	if err := c.storage.SimpleUpload(ctx, jsonPath, createdBytes); err != nil {
		return err
	}
	return nil
}

func (c *Cache) Sync(ctx context.Context, storageID, spaceID string) error {
	var mtime time.Time
	if c.Providers[storageID] != nil && c.Providers[storageID].Spaces[spaceID] != nil {
		mtime = c.Providers[storageID].Spaces[spaceID].Mtime
		//    - y: set If-Modified-Since header to only download if it changed
	} else {
		mtime = time.Time{} // Set zero time so that data from storage always takes precedence
	}

	jsonPath := spaceJSONPath(storageID, spaceID)
	info, err := c.storage.Stat(ctx, jsonPath)
	if err != nil {
		return err
	}
	// check mtime of /users/{userid}/created.json
	if utils.TSToTime(info.Mtime).After(mtime) {
		//  - update cached list of created shares for the user in memory if changed
		createdBlob, err := c.storage.SimpleDownload(ctx, jsonPath)
		if err != nil {
			return err
		}
		newShares := &Shares{}
		err = json.Unmarshal(createdBlob, newShares)
		if err != nil {
			return err
		}
		c.initializeIfNeeded(storageID, spaceID)
		c.Providers[storageID].Spaces[spaceID] = newShares
	}
	return nil
}

func (c *Cache) initializeIfNeeded(storageID, spaceID string) {
	if c.Providers[storageID] == nil {
		c.Providers[storageID] = &Spaces{
			Spaces: map[string]*Shares{},
		}
	}
	if c.Providers[storageID].Spaces[spaceID] == nil {
		c.Providers[storageID].Spaces[spaceID] = &Shares{
			Shares: map[string]*collaboration.Share{},
		}
	}
}

func spaceJSONPath(storageID, spaceID string) string {
	return filepath.Join("/storages", storageID, spaceID+".json")
}
