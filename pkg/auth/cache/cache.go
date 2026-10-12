// Copyright 2018-2026 CERN
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

// Package cache memoizes the outcome of verifying a credential. A sync client
// presents the same secret on every request, which otherwise costs a database
// or identity provider lookup every time. It does not matter what the
// credential is: an app password, a username and password, a public link token
// or a signed JWT all go through the same cache. Rejections are cached too, so
// a client that keeps retrying a stale password is also answered from memory.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/bluele/gcache"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"google.golang.org/protobuf/proto"
)

const (
	defaultSize        = 50000
	defaultTTL         = 300
	defaultNegativeTTL = 30
)

// Embed Config in a service configuration with `mapstructure:",squash"`.
type Config struct {
	Size        int `mapstructure:"cache_size" docs:"50000;Maximum number of authentication outcomes kept in memory."`
	TTL         int `mapstructure:"cache_ttl" docs:"300;Seconds an accepted credential stays cached. An entry always expires at min(TTL, expiration of the credential). A negative value disables the cache."`
	NegativeTTL int `mapstructure:"negative_cache_ttl" docs:"30;Seconds a rejected credential stays cached."`
}

// Token is set only where one was minted for the credential.
type Identity struct {
	User   *userpb.User
	Scopes map[string]*authpb.Scope
	Token  string
}

// Cache is safe for concurrent use.
type Cache struct {
	entries gcache.Cache
	ttl     time.Duration
	negTTL  time.Duration
}

type outcome struct {
	id  *Identity
	err error
}

// New applies the defaults to the zero fields of c. A negative TTL disables
// the cache: it then stores nothing and never reports a hit.
func New(c Config) *Cache {
	if c.TTL < 0 {
		return &Cache{}
	}
	if c.Size <= 0 {
		c.Size = defaultSize
	}
	if c.TTL == 0 {
		c.TTL = defaultTTL
	}
	if c.NegativeTTL == 0 {
		c.NegativeTTL = defaultNegativeTTL
	}
	return &Cache{
		entries: gcache.New(c.Size).LRU().Build(),
		ttl:     time.Duration(c.TTL) * time.Second,
		negTTL:  time.Duration(c.NegativeTTL) * time.Second,
	}
}

// Key hashes the parts, so no secret is held in memory in the clear.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		// Separate the parts, so ("ab", "c") and ("a", "bc") differ.
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the identity, the error that rejected the credential, and
// whether the key was cached at all.
func (c *Cache) Get(key string) (*Identity, error, bool) {
	if c.entries == nil {
		return nil, nil, false
	}
	v, err := c.entries.Get(key)
	if err != nil {
		return nil, nil, false
	}
	o := v.(outcome)
	if o.err != nil {
		return nil, o.err, true
	}
	return clone(o.id), nil, true
}

// Set stores an identity when err is nil, the rejection otherwise. notAfter,
// when it is not the zero time, caps the lifetime of the entry, so a cached
// identity never outlives the credential or the token it describes.
func (c *Cache) Set(key string, id *Identity, err error, notAfter time.Time) {
	if c.entries == nil {
		return
	}

	ttl := c.ttl
	o := outcome{err: err}
	if err != nil {
		ttl = c.negTTL
	} else {
		o.id = clone(id)
	}

	if !notAfter.IsZero() {
		if left := time.Until(notAfter); left < ttl {
			ttl = left
		}
	}
	if ttl <= 0 {
		return
	}

	_ = c.entries.SetWithExpire(key, o, ttl)
}

// clone runs both on the way in and on the way out, because callers modify
// what they get back, for example to fill in the groups of the user.
func clone(id *Identity) *Identity {
	if id == nil {
		return nil
	}
	out := &Identity{Token: id.Token}
	if id.User != nil {
		out.User = proto.Clone(id.User).(*userpb.User)
	}
	if id.Scopes != nil {
		out.Scopes = make(map[string]*authpb.Scope, len(id.Scopes))
		for k, s := range id.Scopes {
			if s == nil {
				out.Scopes[k] = nil
				continue
			}
			out.Scopes[k] = proto.Clone(s).(*authpb.Scope)
		}
	}
	return out
}
