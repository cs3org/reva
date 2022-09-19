Enhancement: allow sharing the gateway caches

We replaced the in memory implementation of the gateway with go-micro stores. `cache_store` can be set to `noop` (default), `memory`, `redis` or `etcd`. The `nats-js` implementation requires a limited set of characters in the key and is currently known to be broken.

The `etag_cache_ttl` was removed as it was not used anyway.

https://github.com/cs3org/reva/pull/3250
