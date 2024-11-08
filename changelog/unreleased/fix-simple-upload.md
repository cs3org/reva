Bugfix: Return correct status codes for simple uploads

Decomposedfs now returns the correct precondition failed status code when the etag does not match. This allows the jsoncs3 share managers optimistic locking to handle concurrent writes correctly

https://github.com/cs3org/reva/pull/4920

