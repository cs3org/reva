Bugfix: Fix trashbin listing with depth 0

The trashbin API handled requests with depth 0 the same as request with a depth of 1. 

https://github.com/cs3org/reva/pull/1956
