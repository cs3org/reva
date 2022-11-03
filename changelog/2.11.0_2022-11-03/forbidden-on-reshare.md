Bugfix: Return OCS forbidden error when a share already exists

We now return OCS 104 / HTTP 403 errors when a user tries to reshare a file with a recipient that already has access to a resource.

https://github.com/cs3org/reva/pull/3287
https://github.com/owncloud/ocis/issues/4630
