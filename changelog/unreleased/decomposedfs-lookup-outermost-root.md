Enhancement: The decomposedfs now returns the outermost space root

When stating a resource by id the decomposedfs will now also return the outermost space root. This allows the gateway to properly reconstruct global paths by combining the space mountpoint and the relative path.
For the owner the root will be the space root, For share recipiens the root will be the outermost shared resource. For now, the root is returned as an opaque property.

https://github.com/cs3org/reva/pull/2373
