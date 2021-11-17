Bugfix: Fix restoring versions

Restoring a version would not remove that version from the version list.
Now the behavior is compatible to ownCloud 10.

https://github.com/owncloud/ocis/issues/1214
https://github.com/cs3org/reva/pull/2270
