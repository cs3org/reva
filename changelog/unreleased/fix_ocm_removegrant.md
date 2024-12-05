Bugfix: Deleting OCM share also updates storageprovider

When remvoving an OCM share we're now also removing the related grant from
the storage provider.

https://github.com/cs3org/reva/pull/4989
https://github.com/owncloud/ocis/issues/10262
