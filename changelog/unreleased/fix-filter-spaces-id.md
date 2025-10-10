Bugfix: fix filtering spaces by ID

Rewrite handling of listing spaces with an ID filter. We now handle one or
multiple filters properly, and the db driver supports querying both by
SpaceID and StorageSpaceID

https://github.com/cs3org/reva/pull/5349
