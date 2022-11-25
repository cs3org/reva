Enhancement: Reduce lock contention issues

We reduced lock contention during high load by caching the extended attributes of a file for the duration of a request.

https://github.com/cs3org/reva/pull/3397
