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

type ActionFunc func()

type Warmuper interface {
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
	cache.SetTTL(c.ttl)

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
