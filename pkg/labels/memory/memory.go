// Copyright 2018-2024 CERN
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

package memory

import (
	"context"
	"errors"
	"sync"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/labels"
	"github.com/cs3org/reva/v3/pkg/labels/registry"
)

func init() {
	registry.Register("memory", New)
}

type mgr struct {
	sync.RWMutex
	favorites map[string]map[string]*provider.ResourceId
}

// New returns an instance of the in-memory favorites manager.
func New(m map[string]any) (labels.Manager, error) {
	return &mgr{favorites: make(map[string]map[string]*provider.ResourceId)}, nil
}

func (m *mgr) ListLabels(ctx context.Context) ([]string, error) {
	return []string{"favorite"}, nil
}

func (m *mgr) ListResourcesForLabel(ctx context.Context, label string) ([]*provider.ResourceId, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("no valid user in context")
	}

	m.RLock()
	defer m.RUnlock()
	favorites := make([]*provider.ResourceId, 0, len(m.favorites[user.Id.OpaqueId]))
	for _, id := range m.favorites[user.Id.OpaqueId] {
		favorites = append(favorites, id)
	}
	return favorites, nil
}

func (m *mgr) SetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errors.New("no valid user in context")
	}

	m.Lock()
	defer m.Unlock()
	if m.favorites[user.Id.OpaqueId] == nil {
		m.favorites[user.Id.OpaqueId] = make(map[string]*provider.ResourceId)
	}
	m.favorites[user.Id.OpaqueId][resourceId.OpaqueId] = resourceId
	return nil
}

func (m *mgr) UnsetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errors.New("no valid user in context")
	}

	m.Lock()
	defer m.Unlock()
	delete(m.favorites[user.Id.OpaqueId], resourceId.OpaqueId)
	return nil
}
