Bugfix: Update space root mtime when changing space metadata

We fixed a problem where space mtimes were not updated when their metadata changed, resulting in changes not being picked up by other services like search.

https://github.com/cs3org/reva/pull/3889
https://github.com/owncloud/ocis/issues/6289
