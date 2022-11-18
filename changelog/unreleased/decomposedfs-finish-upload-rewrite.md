Bugfix: decomposedfs fix revision download

We rewrote the finish upload code to use a write lock when creating and updating node metadata. This prevents some cornercases, allows us to calculate the size diff atomically and fixes downloading revisions.

https://github.com/cs3org/reva/pull/3473