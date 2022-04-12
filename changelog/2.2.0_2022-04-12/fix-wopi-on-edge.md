Bugfix: Fix locking and public link scope checker to make the WOPI server work

We've fixed the locking implementation to use the CS3api instead of the temporary opaque values.
We've fixed the scope checker on public links to allow the OpenInApp actions.

These fixes have been done to use the cs3org/wopiserver with REVA edge.

https://github.com/cs3org/reva/pull/2721
