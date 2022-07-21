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

package lru

import (
	"fmt"
	"time"

	"github.com/bluele/gcache"
	"github.com/cs3org/reva/internal/http/services/thumbnails/cache"
	"github.com/cs3org/reva/internal/http/services/thumbnails/cache/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("lru", New)
}

type lru struct {
	config *config
	cache  gcache.Cache
}

type config struct {
	Size       int `mapstructure:"size"`
	Expiration int `mapstructure:"expiration"`
}

func New(conf map[string]interface{}) (cache.Cache, error) {
	c := &config{}
	err := mapstructure.Decode(conf, c)
	if err != nil {
		return nil, errors.Wrap(err, "lru: error decoding config")
	}
	c.init()

	svc := &lru{
		config: c,
		cache:  gcache.New(c.Size).LRU().Build(),
	}

	return svc, nil
}

func (c *config) init() {
	if c.Size == 0 {
		c.Size = 1000000
	}
	if c.Expiration == 0 {
		c.Expiration = 300
	}
}

func getKey(file, etag string, width, height int) string {
	return fmt.Sprintf("%s:%s:%d:%d", file, etag, width, height)
}

func (l *lru) Get(file, etag string, width, height int) ([]byte, error) {
	key := getKey(file, etag, width, height)
	if value, err := l.cache.Get(key); err == nil {
		return value.([]byte), nil
	}
	return nil, cache.ErrNotFound{}
}

func (l *lru) Set(file, etag string, width, height int, data []byte) error {
	key := getKey(file, etag, width, height)
	return l.cache.SetWithExpire(key, data, time.Duration(l.config.Expiration)*time.Second)
}
