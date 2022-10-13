Bugfix: Fix malformed uid string in cache

The rediscache returns a uid in the format of `<tablename>uid:<someuid>` in the getter
this results in issues when trying to delete the key from the cache store, because
the Delete function will prepend the table name to the string which will not be resolvable in redis
(e.g. `<tablename><tablename>uid:<somuid>`)

https://github.com/cs3org/reva/pull/3338
https://github.com/owncloud/ocis/issues/4772