Bugfix: Handle non uuid space and nodeid in decomposedfs

The decomposedfs no longer panics when trying to look up spaces with a non uuid length id.

https://github.com/cs3org/reva/pull/2854