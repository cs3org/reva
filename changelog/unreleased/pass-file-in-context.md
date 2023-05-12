Bugfix: decomposedfs no longer deadlocks when cache is disabled

We now pass a context to lower level functions which allows passing a reader for already locked files.

https://github.com/cs3org/reva/pull/3885
