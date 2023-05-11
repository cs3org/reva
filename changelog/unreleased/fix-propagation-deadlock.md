Bugfix: decomposedfs no longer deadlocks when cache is disabled

We fixed a bug in the decomposedfs that causes the propagation to deadlock if caching is disabled.

https://github.com/cs3org/reva/pull/3880