Bugfix: Fix Stat() by Path on re-created resource

We fixed bug that caused Stat Requests using a Path reference to a mount point
in the sharejail to not resolve correctly, when a share using the same
mount point to an already deleted resource was still existing.

https://github.com/cs3org/reva/pull/4561
https://github.com/owncloud/ocis/issues/7895
