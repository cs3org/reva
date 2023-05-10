Bugfix: Remove go-micro/store/redis specific workaround

We submitted an upstream fix for an issue in the go-micro/store redis plugin.
Which allowed us to remove a redis specific workaround from the reva storage
cache implementation.

https://github.com/cs3org/reva/pull/3876
