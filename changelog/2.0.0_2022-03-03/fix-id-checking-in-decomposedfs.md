Bugfix: Don't handle ids containing "/" in decomposedfs

The storageprovider previously checked all ids without checking their validity
this lead to flaky test because it shouldn't check ids that are used from the
public storage provider

https://github.com/cs3org/reva/pull/2445

