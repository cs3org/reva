Enhancement: concurrent stat requests when listing shares

The sharesstorageprovider now concurrently stats the accepted shares when listing the share jail. The default number of 5 workers can be changed by setting the `max_concurrency` value in the config map.

https://github.com/cs3org/reva/pull/4812
