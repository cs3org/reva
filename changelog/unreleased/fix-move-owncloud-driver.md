Bugfix: Fix move in owncloud storage driver 

When moving a file or folder (includes renaming) the filepath in the cache didn't get updated which caused subsequent requests to `getpath` to fail.

https://github.com/cs3org/reva/issues/1693
https://github.com/cs3org/reva/issues/1696

