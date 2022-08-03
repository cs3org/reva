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
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
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
}

func New(s metadata.Storage) Cache {
	return Cache{
		Providers: map[string]*Spaces{},
		storage:   s,
	}
}

func (pc *Cache) Add(storageID, spaceID, shareID string, share *collaboration.Share) {
	if pc.Providers[storageID] == nil {
		pc.Providers[storageID] = &Spaces{
			Spaces: map[string]*Shares{},
		}
	}
	if pc.Providers[storageID].Spaces[spaceID] == nil {
		pc.Providers[storageID].Spaces[spaceID] = &Shares{
			Shares: map[string]*collaboration.Share{},
		}
	}
	pc.Providers[storageID].Spaces[spaceID].Shares[shareID] = share
}

func (pc *Cache) Remove(storageID, spaceID, shareID string) {
	if pc.Providers[storageID] == nil ||
		pc.Providers[storageID].Spaces[spaceID] == nil {
		return
	}
	delete(pc.Providers[storageID].Spaces[spaceID].Shares, shareID)
}

func (pc *Cache) Get(storageID, spaceID, shareID string) *collaboration.Share {
	if pc.Providers[storageID] == nil ||
		pc.Providers[storageID].Spaces[spaceID] == nil {
		return nil
	}
	return pc.Providers[storageID].Spaces[spaceID].Shares[shareID]
}

func (pc *Cache) ListSpace(storageID, spaceID string) *Shares {
	if pc.Providers[storageID] == nil {
		return &Shares{}
	}
	return pc.Providers[storageID].Spaces[spaceID]
}
