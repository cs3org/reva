Bugfix: Fix litmus failing on ocis storage

We now ignore the `no data available` error when removing a non existing metadata attribute, which is ok because we are trying to delete it anyway.

https://github.com/cs3org/reva/issues/1178
https://github.com/cs3org/reva/pull/1179