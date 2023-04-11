Bugfix: Fix listing directory for a read-only shares for EOS storage driver

In a read-only share, while listing a folder, for resources
not having a version folder, the returned resource id was wrongly
the one of the original file, instead of the version folder.
This behavior has been fixed, where the version folder is always
created on behalf of the resource owner.

https://github.com/cs3org/reva/pull/3786
