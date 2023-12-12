Bugfix: Fix CS3 status code when looking up non existing share

When trying to lookup a share that does not exist we now return a proper "not found"
error instead of just an "internal error.

https://github.com/cs3org/reva/pull/4366
