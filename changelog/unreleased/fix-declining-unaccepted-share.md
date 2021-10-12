Bugfix: Return OK when trying to delete a non existing reference

When the gateway declines a share we can ignore a non existing reference.

https://github.com/cs3org/reva/pull/2154
https://github.com/owncloud/ocis/pull/2603
