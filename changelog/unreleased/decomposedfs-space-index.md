Enhancement: Improve space index performance

The directory tree based decomposedfs space indexes have been replaced
with messagepack base indexes which improves performance when reading
the index, especially on slow storages.

https://github.com/cs3org/reva/pull/3995
