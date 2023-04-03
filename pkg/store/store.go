// Copyright 2018-2023 CERN
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

package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cs3org/reva/v2/pkg/store/etcd"
	"github.com/cs3org/reva/v2/pkg/store/memory"
	natsjs "github.com/go-micro/plugins/v4/store/nats-js"
	"github.com/go-micro/plugins/v4/store/redis"
	redisopts "github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"github.com/shamaton/msgpack/v2"
	"go-micro.dev/v4/logger"
	microstore "go-micro.dev/v4/store"
)

var ocMemStore *microstore.Store

const (
	TypeMemory        = "memory"
	TypeNoop          = "noop"
	TypeEtcd          = "etcd"
	TypeRedis         = "redis"
	TypeRedisSentinel = "redis-sentinel"
	TypeOCMem         = "ocmem"
	TypeNatsJS        = "nats-js"
)

// Cache handles key value operations on caches
type Cache interface {
	PullFromCache(key string, dest interface{}) error
	PushToCache(key string, src interface{}) error
	List(opts ...microstore.ListOption) ([]string, error)
	Delete(key string, opts ...microstore.DeleteOption) error
	Close() error
}

// Create initializes a new store
func Create(opts ...microstore.Option) microstore.Store {
	options := &microstore.Options{
		Context: context.Background(),
	}
	for _, o := range opts {
		o(options)
	}

	storeType, _ := options.Context.Value(typeContextKey{}).(string)

	switch storeType {
	case TypeNoop:
		return microstore.NewNoopStore(opts...)
	case TypeEtcd:
		return etcd.NewEtcdStore(opts...)
	case TypeRedis:
		// FIXME redis plugin does not support redis cluster or ring -> needs upstream patch or our implementation
		return redis.NewStore(opts...)
	case TypeRedisSentinel:
		redisMaster := ""
		redisNodes := []string{}
		for _, node := range options.Nodes {
			parts := strings.SplitN(node, "/", 2)
			if len(parts) != 2 {
				return nil
			}
			// the first node is used to retrieve the redis master
			redisNodes = append(redisNodes, parts[0])
			if redisMaster == "" {
				redisMaster = parts[1]
			}
		}
		return redis.NewStore(
			microstore.Database(options.Database),
			microstore.Table(options.Table),
			microstore.Nodes(redisNodes...),
			redis.WithRedisOptions(redisopts.UniversalOptions{
				MasterName: redisMaster,
			}),
		)
	case TypeOCMem:
		if ocMemStore == nil {
			var memStore microstore.Store

			sizeNum, _ := options.Context.Value(sizeContextKey{}).(int)
			if sizeNum <= 0 {
				memStore = memory.NewMultiMemStore()
			} else {
				memStore = memory.NewMultiMemStore(
					microstore.WithContext(
						memory.NewContext(
							context.Background(),
							map[string]interface{}{
								"maxCap": sizeNum,
							},
						)),
				)
			}
			ocMemStore = &memStore
		}
		return *ocMemStore
	case TypeNatsJS:
		ttl, _ := options.Context.Value(ttlContextKey{}).(time.Duration)
		// TODO nats needs a DefaultTTL option as it does not support per Write TTL ...
		// FIXME nats has restrictions on the key, we cannot use slashes AFAICT
		// host, port, clusterid
		return natsjs.NewStore(
			append(opts,
				natsjs.NatsOptions(nats.Options{Name: "TODO"}),
				natsjs.DefaultTTL(ttl))...,
		) // TODO test with ocis nats
	case TypeMemory, "mem", "": // allow existing short form and use as default
		return microstore.NewMemoryStore(opts...)
	default:
		// try to log an error
		if options.Logger == nil {
			options.Logger = logger.DefaultLogger
		}
		options.Logger.Logf(logger.ErrorLevel, "unknown store type: '%s', falling back to memory", storeType)
		return microstore.NewMemoryStore(opts...)
	}
}

// CacheStore holds cache store specific configuration
type cacheStore struct {
	s               microstore.Store
	database, table string
	ttl             time.Duration
}

// PullFromCache pulls a value from the configured database and table of the underlying store using the given key
func (cache cacheStore) PullFromCache(key string, dest interface{}) error {
	r, err := cache.s.Read(key, microstore.ReadFrom(cache.database, cache.table), microstore.ReadLimit(1))
	if err != nil {
		return err
	}
	if len(r) == 0 {
		return fmt.Errorf("not found")
	}

	return msgpack.Unmarshal(r[0].Value, &dest)
}

// PushToCache pushes a key and value to the configured database and table of the underlying store
func (cache cacheStore) PushToCache(key string, src interface{}) error {
	b, err := msgpack.Marshal(src)
	if err != nil {
		return err
	}
	return cache.s.Write(
		&microstore.Record{Key: key, Value: b},
		microstore.WriteTo(cache.database, cache.table),
		microstore.WriteTTL(cache.ttl),
	)
}

// List lists the keys on the configured database and table of the underlying store
func (cache cacheStore) List(opts ...microstore.ListOption) ([]string, error) {
	o := []microstore.ListOption{
		microstore.ListFrom(cache.database, cache.table),
	}
	o = append(o, opts...)
	keys, err := cache.s.List(o...)
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		keys[i] = strings.TrimPrefix(key, cache.table)
	}
	return keys, nil
}

// Delete deletes the given key on the configured database and table of the underlying store
func (cache cacheStore) Delete(key string, opts ...microstore.DeleteOption) error {
	o := []microstore.DeleteOption{
		microstore.DeleteFrom(cache.database, cache.table),
	}
	o = append(o, opts...)
	return cache.s.Delete(key, o...)
}

// Close closes the underlying store
func (cache cacheStore) Close() error {
	return cache.s.Close()
}
