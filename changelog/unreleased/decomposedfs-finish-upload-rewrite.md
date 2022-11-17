Bugfix: decomposedfs make finish upload atomic

We rewrote the finish upload code to use a write lock when creating and updating node metadata. This prevents some cornercases and allows us to calculate the size diff atomically.

https://github.com/cs3org/reva/pull/3473