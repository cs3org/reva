Bugfix: Fix error when try to delete space without permission

When a user without the correct permission tries to delete a storage space,
return a PermissionDenied error instead of an Internal Error.

https://github.com/cs3org/reva/pull/3710
