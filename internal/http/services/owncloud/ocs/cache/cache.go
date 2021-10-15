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
	"context"
	"net/http"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"google.golang.org/grpc/metadata"
)

type warmupDriver struct {
	driver Warmuper
	cache  *ttlcache.Cache
}

// A WarmupSet is an object that executes all the warmup strategies
// defined by registering them through the Register method
type WarmupSet struct {
	drivers []*warmupDriver
	ttl     time.Duration
}

// ActionFunc defines the action to run in a warmup strategy
type ActionFunc func()

// A Warmuper is an object that defines a warmup strategy
type Warmuper interface {
	// Warmup is a function that return a function to exec as warmup
	// strategy and a string key that defines when run this function.
	// The object that will execute the function will check in the cache
	// for the key and execute the key only if it not in the cache.
	Warmup(r *http.Request) (string, ActionFunc)
}

// New creates a new CacheWarmup
func New(ttl time.Duration) *WarmupSet {
	return &WarmupSet{
		drivers: make([]*warmupDriver, 0),
		ttl:     ttl,
	}
}

// Register registers a Warmuper
func (c *WarmupSet) Register(w Warmuper) {
	// TODO: check for duplicates
	cache := ttlcache.NewCache()
	_ = cache.SetTTL(c.ttl)

	wd := &warmupDriver{
		driver: w,
		cache:  cache,
	}
	c.drivers = append(c.drivers, wd)
}

// Warmup executes all the warmup strategies registered
func (c *WarmupSet) Warmup(r *http.Request) {
	u := ctxpkg.ContextMustGetUser(r.Context())
	tkn := ctxpkg.ContextMustGetToken(r.Context())

	ctx := context.Background()
	ctx = ctxpkg.ContextSetUser(ctx, u)
	ctx = ctxpkg.ContextSetToken(ctx, tkn)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, tkn)
	req := r.Clone(ctx)

	for _, wd := range c.drivers {
		go func(wd *warmupDriver) {
			key, f := wd.driver.Warmup(req)
			// check if the key is stored in the cache
			if _, err := wd.cache.Get(key); err != nil {
				_ = wd.cache.Set(key, struct{}{})
				f()
			}
		}(wd)
	}
}
