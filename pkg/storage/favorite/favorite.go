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
)

// Manager defines an interface for a favorites manager.
type Manager interface {
	// ListFavorites returns all resources that were favorited by a user.
	ListFavorites(ctx context.Context, userID *user.UserId) ([]*provider.ResourceId, error)
	// SetFavorite marks a resource as favorited by a user.
	SetFavorite(ctx context.Context, userID *user.UserId, resourceID *provider.ResourceId) error
	// UnsetFavorite unmarks a resource as favorited by a user.
	UnsetFavorite(ctx context.Context, userID *user.UserId, resourceID *provider.ResourceId) error
}

// NewInMemoryManager returns an instance of the in-memory favorites manager.
func NewInMemoryManager() Manager {
	return InMemoryManager{favorites: make(map[string]map[string]*provider.ResourceId)}
}

// InMemoryManager implements the Manager interface to manage favorites using an in-memory storage.
// This should not be used in production but can be used for tests.
type InMemoryManager struct {
	favorites map[string]map[string]*provider.ResourceId
}

// ListFavorites returns all resources that were favorited by a user.
func (m InMemoryManager) ListFavorites(ctx context.Context, userID *user.UserId) ([]*provider.ResourceId, error) {
	favorites := make([]*provider.ResourceId, 0, len(m.favorites[userID.OpaqueId]))
	for _, id := range m.favorites[userID.OpaqueId] {
		favorites = append(favorites, id)
	}
	return favorites, nil
}

// SetFavorite marks a resource as favorited by a user.
func (m InMemoryManager) SetFavorite(_ context.Context, userID *user.UserId, resourceID *provider.ResourceId) error {
	if m.favorites[userID.OpaqueId] == nil {
		m.favorites[userID.OpaqueId] = make(map[string]*provider.ResourceId)
	}
	m.favorites[userID.OpaqueId][resourceID.OpaqueId] = resourceID
	return nil
}

// UnsetFavorite unmarks a resource as favorited by a user.
func (m InMemoryManager) UnsetFavorite(_ context.Context, userID *user.UserId, resourceID *provider.ResourceId) error {
	delete(m.favorites[userID.OpaqueId], resourceID.OpaqueId)
	return nil
}
