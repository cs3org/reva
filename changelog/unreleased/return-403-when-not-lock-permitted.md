Bugfix: Return 403 when user is not permitted to log

When a user tries to lock a file, but doesn't have write access, the correct status code is `403` not `500` like we did until now

https://github.com/cs3org/reva/pull/4292
