Bugfix: nilpointer in getPermissionsByCs3Reference

Fix for potential nilpointer: when an err is returned, the status can be nil

https://github.com/cs3org/reva/pull/5380
