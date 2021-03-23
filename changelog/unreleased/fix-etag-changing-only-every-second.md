Bugfix: Fix etag changing only once a second

We fixed a problem with the owncloud storage driver only considering
the mtime with a second resolution for the etag calculation.

https://github.com/cs3org/reva/pull/1576