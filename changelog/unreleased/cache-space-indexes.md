Enhancement: cache space indexes

decomposedfs now caches the different space indexes in memory which greatly improves the performance of ListStorageSpaces on slow storages.

https://github.com/cs3org/reva/pull/3987
