Bugfix: Fix purging deleted files with the ocis storage

The ocis storage could load the owner information of a deleted file. This caused the storage to not be able to purge deleted files.

https://github.com/owncloud/ocis/issues/551
