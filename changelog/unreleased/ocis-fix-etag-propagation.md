Bugfix: Fix etag propagation in ocis driver

We now use a new synctime timestamp instead of trying to read the mtime to avoid race conditions when the stat request happens too quickly.

https://github.com/cs3org/reva/pull/1264
https://github.com/owncloud/product/issues/249