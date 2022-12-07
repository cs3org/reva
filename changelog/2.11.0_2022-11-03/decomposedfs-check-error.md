Bugfix: add missing error check in decomposedfs

During space creation the decomposedfs now checks for errors when trying to read the root node. This prevents a panic by no longer calling InternalPath on the node.

https://github.com/cs3org/reva/pull/3430
https://github.com/owncloud/ocis/issues/4961
