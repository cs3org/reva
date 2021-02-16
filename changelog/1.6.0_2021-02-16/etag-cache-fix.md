Enhancement: Use updated etag of home directory even if it is cached

We cache the home directory and shares folder etags as calculating these is an
expensive process. But if these directories were updated after the previously
calculated etag was cached, we can ignore this calculation and directly return
the new one.

https://github.com/cs3org/reva/pull/1416
