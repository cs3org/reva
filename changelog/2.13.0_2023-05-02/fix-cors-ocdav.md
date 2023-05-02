Bugfix: Fix missing CORS config in ocdav service

The ocdav service is started with a go micro wrapper. We needed to add the cors config.

https://github.com/cs3org/reva/pull/3764
