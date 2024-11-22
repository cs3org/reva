Bugfix: make uploads over gRPC work

Uploads would return a 500 error, even though they succeeded, due to a failed Stat after an upload. The Stat failed because it was executed as cbox, which no longer has access to user files. This is fixed by using the user's auth now instead.

https://github.com/cs3org/reva/pull/4969