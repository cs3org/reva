Bugfix: Fix trashbin purge

We have fixed a nil-pointer-exception, when purging files from the trashbin that do not have a parent (any more)

https://github.com/cs3org/reva/pull/3857
https://github.com/owncloud/ocis/issues/6245