Bugfix: correct share jail child aggregation

We now add up the size of all mount points when aggregating the size for a child with the same name. Furthermore, the listing should no longer contain duplicate entries.

https://github.com/cs3org/reva/pull/2907