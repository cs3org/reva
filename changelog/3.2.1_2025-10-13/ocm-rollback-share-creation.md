Bugfix: Rollback OCM share creation if remote request fails

The OCM share request to the remote server happens after creating the share locally
this means that if the remote request fails it should be rolled back (e.g. delete the share).

https://github.com/cs3org/reva/pull/5351
