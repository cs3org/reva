Bugfix: decomposedfs propagate sizediff

We now propagate the size diff instead of calculating the treesize. This fixes the slower upload speeds in large folders.

https://github.com/cs3org/reva/pull/3482
https://github.com/owncloud/ocis/issues/5061