Bugfix: Fix expired authenticated public link error code

On an expired authenticated public link, the error returned
was 401 unauthorized, behaving differently from
a not-authenticated one, that returns 404 not found.
This has been fixed, returning 404 not found.

https://github.com/cs3org/reva/pull/3528