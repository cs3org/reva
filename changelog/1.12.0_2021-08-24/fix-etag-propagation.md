Bugfix: Fix etag propagation on deletes

When deleting a file the etag propagation would skip the parent of the deleted file.

https://github.com/cs3org/reva/pull/1957
