Enhancement: add quota cache

Since EOS at the moment takes a long time to respond to `GetQuota` requests, 
we have implemented a cache for the quota results. The cache is warmed up on
init, where the cache is populated from a single `GetQuota` call that obtains all user quotas.

https://github.com/cs3org/reva/pull/5557