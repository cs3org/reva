Bugfix: Fix definition of "file-editor" role

The "file-editor" role was missing the AddGrant resource permission, which
caused a broken mapping from ResourcePermissions to roles in certain cases.

https://github.com/cs3org/reva/pull/4336
