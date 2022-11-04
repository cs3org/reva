Bugfix: Fix possible race conditions

We fixed two potential race condition when initializing the shared config
structure and when setting up caches for the http authentication interceptors.

https://github.com/cs3org/reva/pull/3377
