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

import (
	"encoding/json"
	"fmt"
	"time"

	natsjs "github.com/go-micro/plugins/v4/store/nats-js"
	"github.com/go-micro/plugins/v4/store/redis"
	"github.com/nats-io/nats.go"
	microetcd "github.com/owncloud/ocis/v2/ocis-pkg/store/etcd"
	microstore "go-micro.dev/v4/store"
)

// Caches holds all caches used by the gateway
type Caches struct {
	Stat       StatCache
	Provider   ProviderCache
	CreateHome CreateHomeCache
}

// Close closes all caches and return only the first error
func (caches *Caches) Close() error {
	err1 := caches.Stat.Close()
	err2 := caches.Provider.Close()
	err3 := caches.CreateHome.Close()
	switch {
	case err1 != nil:
		return err1
	case err2 != nil:
		return err2
	case err3 != nil:
		return err3
	}
	return nil
}

// Cache holds cache specific configuration
type Cache struct {
	s   microstore.Store
	ttl time.Duration
}

// NewCache initializes a cache
func NewCache(store string, nodes []string, ttl time.Duration) Cache {
	return Cache{
		s:   getStore(store, nodes, ttl), // some stores use a default ttl so we pass it when initializing
		ttl: ttl,                         // some stores use the ttl on every write, so we remember it here
	}
}

// NewCaches initializes the caches.
func NewCaches(cacheStore string, cacheNodes []string, statTTL, providerTTL, createHomeTTL time.Duration) Caches {
	c := Caches{}

	if statTTL > 0 {
		c.Stat = NewStatCache(cacheStore, cacheNodes, statTTL)
	} else {
		c.Stat = NewStatCache("noop", []string{}, 0)
	}

	if providerTTL > 0 {
		c.Provider = NewProviderCache(cacheStore, cacheNodes, providerTTL)
	} else {
		c.Provider = NewProviderCache("noop", []string{}, 0)
	}

	if createHomeTTL > 0 {

		c.CreateHome = NewCreateHomeCache(cacheStore, cacheNodes, createHomeTTL)
	} else {
		c.CreateHome = NewCreateHomeCache("noop", []string{}, 0)
	}

	return c
}

func getStore(store string, nodes []string, ttl time.Duration) microstore.Store {
	switch store {
	case "etcd":
		return microetcd.NewEtcdStore(
			microstore.Nodes(nodes...),
		)
	case "nats-js":
		// TODO nats needs a DefaultTTL option as it does not support per Write TTL ...
		// FIXME nats has restrictions on the key, we cannot use slashes AFAICT
		// host, port, clusterid
		return natsjs.NewStore(
			microstore.Nodes(nodes...),
			natsjs.NatsOptions(nats.Options{Name: "TODO"}),
			natsjs.DefaultTTL(ttl),
		) // TODO test with ocis nats
	case "redis":
		// FIXME redis plugin does not support redis cluster, sentinel or ring -> needs upstream patch or our implementation
		return redis.NewStore(
			microstore.Nodes(nodes...),
		) // only the first node is taken into account
	case "memory":
		return microstore.NewStore()
	default:
		return microstore.NewNoopStore()
	}
}

func (cache *Cache) PullFromCache(key string, dest interface{}) error {
	r, err := cache.s.Read(key, microstore.ReadLimit(1))
	if err != nil {
		return err
	}
	if len(r) == 0 {
		return fmt.Errorf("not found")
	}
	return json.Unmarshal(r[0].Value, dest)
}

func (cache *Cache) PushToCache(key string, src interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return cache.s.Write(
		&microstore.Record{Key: key, Value: b},
		microstore.WriteTTL(cache.ttl),
	)
}

func (cache *Cache) List(opts ...microstore.ListOption) ([]string, error) {
	return cache.s.List(opts...)
}

func (cache *Cache) Delete(key string, opts ...microstore.DeleteOption) error {
	return cache.s.Delete(key, opts...)
}

func (cache *Cache) Close() error {
	return cache.s.Close()
}
