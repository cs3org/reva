Bugfix: Handle more eos errors

We now treat E2BIG, EACCES as a permission error, which occur, eg. when acl checks fail and return a permission denied error.

https://github.com/cs3org/reva/pull/1269