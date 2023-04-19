Enhancement: Cache node ids

decomposedfs now keeps an in-memory cache for node ids, sparing a lot of ReadLink calls.

https://github.com/cs3org/reva/pull/3801
