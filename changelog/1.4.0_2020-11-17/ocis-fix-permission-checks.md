Bugfix: Check current node when iterating over path segments

When checking permissions we were always checking the leaf instead of using the current node while iterating over path segments.

https://github.com/cs3org/reva/pull/1255
