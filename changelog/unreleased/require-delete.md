Bugfix: Require Delete for move permission

The ace handler would give a `Move` permission if the user had upload & download. This is not correct however. You need the `Delete` permissions too to be able to move.

https://github.com/owncloud/reva/pull/294
