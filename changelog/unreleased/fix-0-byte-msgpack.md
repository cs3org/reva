Bugfix: fix writing 0 byte msgpack metadata

File metadata is now written atomically to be more resilient during timeouts

https://github.com/cs3org/reva/pull/4042
https://github.com/cs3org/reva/pull/4034
https://github.com/cs3org/reva/pull/4033
