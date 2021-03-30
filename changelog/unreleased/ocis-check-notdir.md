Bugfix: check for ENOTDIR on readlink error

The deconstructed storage driver now handles ENOTDIR errors when `node.Child()` is called for a path containing a path segment that is actually a file.

https://github.com/cs3org/reva/pull/1597
https://github.com/owncloud/ocis/issues/1239
