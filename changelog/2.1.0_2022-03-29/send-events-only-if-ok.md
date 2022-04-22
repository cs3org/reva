Bugfix: send events only if response code is `OK`

Before events middleware was sending events also when the resulting status code was not `OK`. This is clearly a bug.

https://github.com/cs3org/reva/pull/2621
