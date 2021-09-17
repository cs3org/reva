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

package favorite

import (
	"context"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
)

// Manager defines an interface for a favorites manager.
type Manager interface {
	// ListFavorites returns all resources that were favorited by a user.
	ListFavorites(ctx context.Context, userID *user.UserId) ([]*provider.ResourceInfo, error)
	// SetFavorite marks a resource as favorited by a user.
	SetFavorite(ctx context.Context, userID *user.UserId, resourceID *provider.ResourceId) error
	// UnsetFavorite unmarks a resource as favorited by a user.
	UnsetFavorite(ctx context.Context, userID *user.UserId, resourceID *provider.ResourceId) error
}

// NewInMemoryManager returns an instance of a favorites manager using an in-memory storage.
func NewInMemoryManager(fs storage.FS) Manager {
	return InMemoryManager{fs: fs, favorites: make(map[string]map[string]struct{})}
}

// InMemoryManager implements the Manager interface to manage favorites using an in-memory storage.
type InMemoryManager struct {
	fs        storage.FS
	favorites map[string]map[string]struct{}
}

// ListFavorites returns all resources that were favorited by a user.
func (m InMemoryManager) ListFavorites(ctx context.Context, userID *user.UserId) ([]*provider.ResourceInfo, error) {
	favorites := make([]*provider.ResourceInfo, 0, len(m.favorites[userID.OpaqueId]))
	for id := range m.favorites[userID.OpaqueId] {
		info, err := m.fs.GetMD(ctx, &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: id}}, []string{})
		if err != nil {
			continue
		}
		favorites = append(favorites, info)
	}
	return favorites, nil
}

// SetFavorite marks a resource as favorited by a user.
func (m InMemoryManager) SetFavorite(_ context.Context, userID *user.UserId, resourceID *provider.ResourceId) error {
	if m.favorites[userID.OpaqueId] == nil {
		m.favorites[userID.OpaqueId] = make(map[string]struct{})
	}
	m.favorites[userID.OpaqueId][resourceID.OpaqueId] = struct{}{}

	return nil
}

// UnsetFavorite unmarks a resource as favorited by a user.
func (m InMemoryManager) UnsetFavorite(_ context.Context, userID *user.UserId, resourceID *provider.ResourceId) error {
	delete(m.favorites[userID.OpaqueId], resourceID.OpaqueId)

	return nil
}
