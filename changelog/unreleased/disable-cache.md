Bugfix: Disable caching

We disabled the cache, because there are race condtions that cause tests to fail.

https://github.com/owncloud/ocis/issues/4251
https://github.com/cs3org/reva/pull/3167
