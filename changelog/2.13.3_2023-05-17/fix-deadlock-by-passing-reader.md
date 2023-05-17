Bugfix: decomposedfs no longer deadlocks when cache is disabled

We now pass a Reader for the locked file to lower level functions so metadata can be read without aquiring a new file lock.

https://github.com/cs3org/reva/pull/3886
