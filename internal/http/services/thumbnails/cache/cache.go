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

// Cache is the interface for a thumbnail cache
type Cache interface {
	// Get gets the thumbnail if stored in the cache
	Get(file, etag string, width, height int) ([]byte, error)
	// Set adds the thumbnail in the cache
	Set(file, etag string, width, height int, data []byte) error
}

type noCache struct{}

// NewNoCache creates a dummy cache that does not cache anything
func NewNoCache() Cache {
	return noCache{}
}

// Get on a NoCache always return ErrNotFound
func (noCache) Get(_, _ string, _, _ int) ([]byte, error) {
	return nil, ErrNotFound{}
}

// Set on a NoCache just does not save the thumbnail
func (noCache) Set(_, _ string, _, _ int, _ []byte) error {
	return nil
}
