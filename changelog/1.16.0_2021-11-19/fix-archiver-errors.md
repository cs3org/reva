Bugfix: Fix HTTP return code when path is invalid

Before when a path was invalid, the archiver returned a
500 error code.
Now this is fixed and returns a 404 code.

https://github.com/cs3org/reva/pull/2294