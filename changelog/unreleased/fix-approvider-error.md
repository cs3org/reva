Bugfix: Fix a wrong error code when approvider creates a new file

We fixed a problem where the approvider would return a 500 error instead of 403 when trying to create a new file in a read-only share.

https://github.com/cs3org/reva/pull/4964
