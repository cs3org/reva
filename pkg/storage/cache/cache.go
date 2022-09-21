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
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	natsjs "github.com/go-micro/plugins/v4/store/nats-js"
	"github.com/go-micro/plugins/v4/store/redis"
	"github.com/nats-io/nats.go"
	microetcd "github.com/owncloud/ocis/v2/ocis-pkg/store/etcd"
	microstore "go-micro.dev/v4/store"
)

var (
	// DefaultStatCache is the memory store.
	statCaches       map[string]StatCache       = make(map[string]StatCache)
	providerCaches   map[string]ProviderCache   = make(map[string]ProviderCache)
	createHomeCaches map[string]CreateHomeCache = make(map[string]CreateHomeCache)
	mutex            sync.Mutex
)

type Cache interface {
	PullFromCache(key string, dest interface{}) error
	PushToCache(key string, src interface{}) error
	List(opts ...microstore.ListOption) ([]string, error)
	Delete(key string, opts ...microstore.DeleteOption) error
	Close() error
}
type StatCache interface {
	Cache
	RemoveStat(userID *userpb.UserId, res *provider.ResourceId)
	GetKey(userID *userpb.UserId, ref *provider.Reference, metaDataKeys, fieldMaskPaths []string) string
}
type ProviderCache interface {
	Cache
	RemoveListStorageProviders(res *provider.ResourceId)

	GetKey(userID *userpb.UserId, spaceID string) string
}

type CreateHomeCache interface {
	Cache
	RemoveCreateHome(res *provider.ResourceId)
	GetKey(userID *userpb.UserId) string
}

func GetStatCache(cacheStore string, cacheNodes []string, database, table string, TTL time.Duration) StatCache {
	mutex.Lock()
	defer mutex.Unlock()

	key := strings.Join(append(append([]string{cacheStore}, cacheNodes...), database, table), ":")
	if statCaches[key] == nil {
		statCaches[key] = NewStatCache(cacheStore, cacheNodes, database, table, TTL)
	}
	return statCaches[key]
}

func GetProviderCache(cacheStore string, cacheNodes []string, database, table string, TTL time.Duration) ProviderCache {
	mutex.Lock()
	defer mutex.Unlock()

	key := strings.Join(append(append([]string{cacheStore}, cacheNodes...), database, table), ":")
	if providerCaches[key] == nil {
		providerCaches[key] = NewProviderCache(cacheStore, cacheNodes, database, table, TTL)
	}
	return providerCaches[key]
}

func GetCreateHomeCache(cacheStore string, cacheNodes []string, database, table string, TTL time.Duration) CreateHomeCache {
	mutex.Lock()
	defer mutex.Unlock()

	key := strings.Join(append(append([]string{cacheStore}, cacheNodes...), database, table), ":")
	if createHomeCaches[key] == nil {
		createHomeCaches[key] = NewCreateHomeCache(cacheStore, cacheNodes, database, table, TTL)
	}
	return createHomeCaches[key]
}

// CacheStore holds cache store specific configuration
type CacheStore struct {
	s               microstore.Store
	database, table string
	ttl             time.Duration
}

// NewCache initializes a new CacheStore
func NewCache(store string, nodes []string, database, table string, ttl time.Duration) Cache {
	return CacheStore{
		s:        getStore(store, nodes, database, table, ttl), // some stores use a default ttl so we pass it when initializing
		database: database,
		table:    table,
		ttl:      ttl, // some stores use the ttl on every write, so we remember it here
	}
}

func getStore(store string, nodes []string, database, table string, ttl time.Duration) microstore.Store {
	switch store {
	case "etcd":
		return microetcd.NewEtcdStore(
			microstore.Nodes(nodes...),
			microstore.Database(database),
			microstore.Table(table),
		)
	case "nats-js":
		// TODO nats needs a DefaultTTL option as it does not support per Write TTL ...
		// FIXME nats has restrictions on the key, we cannot use slashes AFAICT
		// host, port, clusterid
		return natsjs.NewStore(
			microstore.Nodes(nodes...),
			microstore.Database(database),
			microstore.Table(table),
			natsjs.NatsOptions(nats.Options{Name: "TODO"}),
			natsjs.DefaultTTL(ttl),
		) // TODO test with ocis nats
	case "redis":
		// FIXME redis plugin does not support redis cluster, sentinel or ring -> needs upstream patch or our implementation
		return redis.NewStore(
			microstore.Database(database),
			microstore.Table(table),
			microstore.Nodes(nodes...),
		) // only the first node is taken into account
	case "memory":
		return microstore.NewStore(
			microstore.Database(database),
			microstore.Table(table),
		)
	default:
		return microstore.NewNoopStore(
			microstore.Database(database),
			microstore.Table(table),
		)
	}
}

func (cache CacheStore) PullFromCache(key string, dest interface{}) error {
	r, err := cache.s.Read(key, microstore.ReadFrom(cache.database, cache.table), microstore.ReadLimit(1))
	if err != nil {
		return err
	}
	if len(r) == 0 {
		return fmt.Errorf("not found")
	}
	return json.Unmarshal(r[0].Value, dest)
}

func (cache CacheStore) PushToCache(key string, src interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return cache.s.Write(
		&microstore.Record{Key: key, Value: b},
		microstore.WriteTo(cache.database, cache.table),
		microstore.WriteTTL(cache.ttl),
	)
}

func (cache CacheStore) List(opts ...microstore.ListOption) ([]string, error) {
	o := []microstore.ListOption{
		microstore.ListFrom(cache.database, cache.table),
	}
	o = append(o, opts...)
	return cache.s.List(o...)
}

func (cache CacheStore) Delete(key string, opts ...microstore.DeleteOption) error {
	o := []microstore.DeleteOption{
		microstore.DeleteFrom(cache.database, cache.table),
	}
	o = append(o, opts...)
	return cache.s.Delete(key, o...)
}

func (cache CacheStore) Close() error {
	return cache.s.Close()
}
