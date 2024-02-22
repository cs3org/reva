Bugfix: Prevent setting container specific permissions on files

It was possible to set the 'CreateContainer', 'Move' or 'Delete' permissions on
file resources with a CreateShare request. These permissions are meant to be only
set on container resources. The UpdateShare request already has a similar check.

https://github.com/cs3org/reva/pull/4462
https://github.com/owncloud/ocis/issues/8131
