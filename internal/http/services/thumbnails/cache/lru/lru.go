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
