Enhancement: Replace gcache with ttlcache

Every idle revad uses about 400 MB, because gcache allocates the full map of a 
cache when it is built. With a size of 1,000,000, one empty cache takes about 
56 MB. This PR moves these caches to ttlcache, which grows with the entries.

https://github.com/cs3org/reva/pull/6055
