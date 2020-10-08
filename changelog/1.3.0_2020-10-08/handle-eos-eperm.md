Bugfix: Handle eos EPERM as permission denied

We now treat EPERM errors, which occur, eg. when acl checks fail and return a permission denied error.

https://github.com/cs3org/reva/pull/1183