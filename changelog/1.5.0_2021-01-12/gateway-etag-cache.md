Enhancement: Add cache for calculated etags for home and shares directory

Since we store the references in the shares directory instead of actual
resources, we need to calculate the etag on every list/stat call. This is rather
expensive so adding a cache would help to a great extent with regard to the
performance.

https://github.com/cs3org/reva/pull/1359
