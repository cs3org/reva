Enhancement: Refactored the xattrs package in the decomposedfs

The xattrs package now uses the xattr.ENOATTR instead of os.ENODATA or os.ENOATTR to check attribute existence.

https://github.com/cs3org/reva/pull/2540
