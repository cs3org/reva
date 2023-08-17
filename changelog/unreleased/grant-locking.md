Bugfix: fix jsoncs3 atomic persistence

The jsoncs3 shame manager now uses etags instead of mtimes to determine when metadata needs to be updated.
As a precondtition we had to change decomposedfs as well: to consistently calculate the etag for the file content
we now store the mtime in the metadata and use the metadata lock for atomicity.

https://github.com/cs3org/reva/pull/4117
