Enhancement: allow sharing the gateway caches

We replaced the in memory implementation of the gateway with go-micro stores. The gateways `cache_store` defaults to `noop` and can be set to `memory`, `redis` or `etcd`. When setting it also set any dataproviders `datatxs.*.cache_store` new config option to the same values so they can invalidate the cache when a file has been uploadad.

Cache instances will be shared between handlers when they use the same configuration in the same process to allow the dataprovider to access the same cache as the gateway.

The `nats-js` implementation requires a limited set of characters in the key and is currently known to be broken.

The `etag_cache_ttl` was removed as it was not used anyway.

https://github.com/cs3org/reva/pull/3250
