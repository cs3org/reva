Bugfix: Remove fmt.Print statement from decomposedfs

The logs have been spammed with messages like `could not create symlink for ...` when using the decomposedfs, eg. with the oCIS storage.

https://github.com/cs3org/reva/pull/1988
