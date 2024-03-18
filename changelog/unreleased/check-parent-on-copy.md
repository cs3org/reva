Bugfix: Prevent copying a file to a parent folder

When copying a file to its parent folder, the file would be copied onto the parent folder, moving the original folder to the trash-bin.

https://github.com/cs3org/reva/pull/4584
https://github.com/cs3org/reva/pull/4582
https://github.com/cs3org/reva/pull/4571
https://github.com/owncloud/ocis/issues/1230