Bugfix: Do not try to use the cache for empty node

We fixed a problem where nodes that did not have an ID set were still trying to use the cache for their metadata resulting in clashing cache keys.

https://github.com/cs3org/reva/pull/3902
