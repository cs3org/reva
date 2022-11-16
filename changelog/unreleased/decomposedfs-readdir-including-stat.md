Enhancement: more efficient tree size calculation in decomposedfs

We now fetch the stat info of all children using `Readdir` instead of `Readdirnames` and subsequent `Stat` calls. While it does not remove the tree size aggregation penalty during uploads it does help in large directories.

https://github.com/cs3org/reva/pull/3468
https://github.com/owncloud/ocis/issues/5061