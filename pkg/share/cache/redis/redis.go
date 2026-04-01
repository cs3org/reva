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

package redis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/cache"
	"github.com/cs3org/reva/v3/pkg/share/cache/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("redis", New[*provider.ResourceInfo])
	registry.Register("redis_space", New[*provider.StorageSpace])
}

type config struct {
	RedisAddress      string `mapstructure:"redis_address"`
	RedisUsername     string `mapstructure:"redis_username"`
	RedisPassword     string `mapstructure:"redis_password"`
	RedisMasterName   string `mapstructure:"redis_master_name"`
	RedisSentinelMode bool   `mapstructure:"redis_sentinel_mode"`
}

type manager[T cache.Cacheable] struct {
	redisPool *redis.Pool
}

func (c *config) ApplyDefaults() {
	if c.RedisAddress == "" {
		c.RedisAddress = "localhost:6379"
	}
	if c.RedisMasterName == "" {
		c.RedisMasterName = "cboxmaster"
	}
	if !c.RedisSentinelMode {
		c.RedisSentinelMode = false
	}
}

// New returns an implementation of a resource info cache that stores the objects in a redis cluster.
func New[T cache.Cacheable](m map[string]any) (cache.GenericCache[T], error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	pool := &redis.Pool{
		MaxIdle:     50,
		MaxActive:   1000,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			var opts []redis.DialOption
			// Only authenticate if a password is configured.
			// (Redis will error if AUTH is attempted but no password is set server-side.)
			if c.RedisPassword != "" {
				if c.RedisUsername != "" {
					opts = append(opts, redis.DialUsername(c.RedisUsername))
				}
				opts = append(opts, redis.DialPassword(c.RedisPassword))
			}

			address := c.RedisAddress
			sentinelMode := c.RedisSentinelMode
			masterName := c.RedisMasterName

			// Non-sentinel mode: connect directly.
			if !sentinelMode {
				return redis.Dial("tcp", address, opts...)
			}

			if masterName == "" {
				return nil, errors.New("cache: redis_sentinel_mode enabled but redis_master_name is empty")
			}

			// Sentinel mode.
			// We treat `address` as a sentinel endpoint. If it turns out not to be a sentinel
			// (eg points to a regular redis server), fall back to direct dialing.
			sentinelConn, err := redis.Dial("tcp", address, opts...)
			if err != nil {
				return nil, err
			}
			defer sentinelConn.Close()

			reply, err := redis.Values(sentinelConn.Do("SENTINEL", "get-master-addr-by-name", masterName))
			if err != nil {
				// If we connected to a plain redis instance, it will reply with "unknown command 'SENTINEL'".
				// In that case, just use the configured address directly.
				msg := strings.ToLower(err.Error())
				if strings.Contains(msg, "unknown command") && strings.Contains(msg, "sentinel") {
					return redis.Dial("tcp", address, opts...)
				}
				return nil, err
			}
			if len(reply) != 2 {
				return nil, errors.New("cache: invalid sentinel reply for get-master-addr-by-name")
			}

			host, err := redis.String(reply[0], nil)
			if err != nil {
				return nil, err
			}
			port, err := redis.String(reply[1], nil)
			if err != nil {
				return nil, err
			}

			masterAddr := fmt.Sprintf("%s:%s", host, port)
			return redis.Dial("tcp", masterAddr, opts...)
		},

		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &manager[T]{
		redisPool: pool,
	}, nil
}

func (m *manager[T]) Get(key string) (T, error) {
	infos, err := m.getVals([]string{key})
	if err != nil {
		var zero T
		return zero, err
	}
	return infos[0], nil
}

func (m *manager[T]) GetKeys(keys []string) ([]T, error) {
	return m.getVals(keys)
}

func (m *manager[T]) Set(key string, info T) error {
	return m.setVal(key, info, -1)
}

func (m *manager[T]) SetWithExpire(key string, info T, expiration time.Duration) error {
	return m.setVal(key, info, int(expiration.Seconds()))
}

func (m *manager[T]) setVal(key string, info T, expiration int) error {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		encodedInfo, err := json.Marshal(&info)
		if err != nil {
			return err
		}

		args := []any{key, encodedInfo}
		if expiration != -1 {
			args = append(args, "EX", expiration)
		}

		if _, err := conn.Do("SET", args); err != nil {
			return err
		}
		return nil
	}
	return errors.New("cache: unable to get connection from redis pool")
}

func (m *manager[T]) getVals(keys []string) ([]T, error) {
	conn := m.redisPool.Get()
	defer conn.Close()

	if conn != nil {
		vals, err := redis.Strings(conn.Do("MGET", keys))
		if err != nil {
			return nil, err
		}

		var zero T
		infos := make([]T, len(keys))
		for i, v := range vals {
			if v != "" {
				if err = json.Unmarshal([]byte(v), &infos[i]); err != nil {
					infos[i] = zero
				}
			}
		}
		return infos, nil
	}
	return nil, errors.New("cache: unable to get connection from redis pool")
}
