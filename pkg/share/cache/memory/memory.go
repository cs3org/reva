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
	"time"

	"github.com/bluele/gcache"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/cache"
	"github.com/cs3org/reva/v3/pkg/share/cache/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	registry.Register("memory", New[*provider.ResourceInfo])
	registry.Register("memory_space", New[*provider.StorageSpace])

}

type config struct {
	CacheSize int `mapstructure:"cache_size"`
}

type manager[T cache.Cacheable] struct {
	cache gcache.Cache
}

func (c *config) ApplyDefaults() {
	if c.CacheSize == 0 {
		c.CacheSize = 1000000
	}
}

// New returns an implementation of a resource info cache that stores the objects in memory.
func New[T cache.Cacheable](m map[string]any) (cache.GenericCache[T], error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &manager[T]{
		cache: gcache.New(c.CacheSize).LFU().Build(),
	}, nil
}

func (m *manager[T]) Get(key string) (T, error) {
	var zero T
	infoIf, err := m.cache.Get(key)
	if err != nil {
		return zero, err
	}
	return infoIf.(T), nil
}

func (m *manager[T]) GetKeys(keys []string) ([]T, error) {
	infos := make([]T, len(keys))
	for i, key := range keys {
		if infoIf, err := m.cache.Get(key); err == nil {
			infos[i] = infoIf.(T)
		}
	}
	return infos, nil
}

func (m *manager[T]) Set(key string, info T) error {
	return m.cache.Set(key, info)
}

func (m *manager[T]) SetWithExpire(key string, info T, expiration time.Duration) error {
	return m.cache.SetWithExpire(key, info, expiration)
}
