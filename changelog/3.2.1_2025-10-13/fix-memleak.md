Bugfix: add user filter to OCS listSharesWithOthers

Fixes a memory leak where an OCS function would load all shares, because no filter for the current user was set

https://github.com/cs3org/reva/pull/5333
