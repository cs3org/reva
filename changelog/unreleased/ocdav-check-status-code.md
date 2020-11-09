Bugfix: check the err and the response status code

The publicfile handler needs to check the response status code to return proper not pound and permission errors in the webdav api.

https://github.com/cs3org/reva/pull/1297
