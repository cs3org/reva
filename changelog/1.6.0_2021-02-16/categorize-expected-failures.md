Enhancement: We categorized the list of expected failures

We categorized all expected failures into _File_ (Basic file management like up and download, move, copy, properties, trash, versions and chunking), _Sync_ (Synchronization features like etag propagation, setting mtime and locking files), _Share_ (File and sync features in a shared scenario), _User management_ (User and group management features) and _Other_ (API, search, favorites, config, capabilities, not existing endpoints, CORS and others). The [Review and fix the tests that have sharing step to work with ocis](https://github.com/owncloud/core/issues/38006) reference has been removed, as we now have the sharing category

https://github.com/cs3org/reva/pull/1424
https://github.com/owncloud/core/issues/38006