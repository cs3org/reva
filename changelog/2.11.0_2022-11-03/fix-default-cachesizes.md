Bugfix: Reduced default cache sizes for smaller memory footprint

We reduced the default cachesizes of the auth interceptors and the share
cache. The default of 1 Million cache entries was way too high and caused
a high memory usage upon startup. Config options to set custom cache size
where added.

https://github.com/owncloud/ocis/issues/3267
https://github.com/owncloud/ocis/issues/4628
