Security: reject dot segments in HTTP request paths

Requests with "." or ".." path segments now get 400 before auth runs, so they
can no longer pass an unprotected prefix and then reach a protected handler.

https://github.com/cs3org/reva/pull/5877
