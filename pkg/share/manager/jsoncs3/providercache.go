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

import collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"

type ProviderCache struct {
	Providers map[string]*ProviderSpaces
}

type ProviderSpaces struct {
	Spaces map[string]*ProviderShares
}

type ProviderShares struct {
	Shares map[string]*collaboration.Share
}

func NewProviderCache() ProviderCache {
	return ProviderCache{
		Providers: map[string]*ProviderSpaces{},
	}
}

func (pc *ProviderCache) Add(storageID, spaceID, shareID string, share *collaboration.Share) {
	if pc.Providers[storageID] == nil {
		pc.Providers[storageID] = &ProviderSpaces{
			Spaces: map[string]*ProviderShares{},
		}
	}
	if pc.Providers[storageID].Spaces[spaceID] == nil {
		pc.Providers[storageID].Spaces[spaceID] = &ProviderShares{
			Shares: map[string]*collaboration.Share{},
		}
	}
	pc.Providers[storageID].Spaces[spaceID].Shares[shareID] = share
}

func (pc *ProviderCache) Remove(storageID, spaceID, shareID string) {
	if pc.Providers[storageID] == nil ||
		pc.Providers[storageID].Spaces[spaceID] == nil {
		return
	}
	delete(pc.Providers[storageID].Spaces[spaceID].Shares, shareID)
}

func (pc *ProviderCache) Get(storageID, spaceID, shareID string) *collaboration.Share {
	if pc.Providers[storageID] == nil ||
		pc.Providers[storageID].Spaces[spaceID] == nil {
		return nil
	}
	return pc.Providers[storageID].Spaces[spaceID].Shares[shareID]
}

func (pc *ProviderCache) ListSpace(storageID, spaceID string) *ProviderShares {
	if pc.Providers[storageID] == nil {
		return &ProviderShares{}
	}
	return pc.Providers[storageID].Spaces[spaceID]
}
