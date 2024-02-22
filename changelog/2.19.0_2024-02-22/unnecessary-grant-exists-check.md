Bugfix: drop unnecessary grant exists check

At least the jsoncs3 share manager properly returns an ALREADY_EXISTS response when trying to add a share to a resource that has already been shared with the grantee.

https://github.com/cs3org/reva/pull/4530