Bugfix: Fix ListStorageSpaces for OCM shares

There are no mount points for OCM shares currently. So the ListStorageSpaces
does not return any for them anymore.

https://github.com/cs3org/reva/pull/4986
https://github.com/owncloud/ocis/issues/10689
