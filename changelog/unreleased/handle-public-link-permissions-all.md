Bugfix: handle sending all permissions when creating public links

For backwards compatability we now reduce permissions to readonly when a create public link carries all permissions.

https://github.com/cs3org/reva/issues/2336
https://github.com/owncloud/ocis/issues/1269