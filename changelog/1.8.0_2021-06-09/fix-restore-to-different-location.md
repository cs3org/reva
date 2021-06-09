Bugfix: Allow for restoring recycle items to different locations

The CS3 APIs specify a way to restore a recycle item to a different location
than the original by setting the `restore_path` field in the
`RestoreRecycleItemRequest`. This field had not been considered until now.

https://github.com/cs3org/reva/pull/1541
https://cs3org.github.io/cs3apis/