Bugfix: fix OCM legacy access

This PR introduces a test to access a remote OCM endpoint
via basic auth (OCM v1.0) and the corresponding implementation
in the DAV client and server to deal with such accesses.
Several log lines on all OCM interactions have been added.

https://github.com/cs3org/reva/pull/5338
