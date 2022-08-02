Bugfix: Check if symlink exists instead of spamming the console

The logs have been spammed with messages like `could not create symlink for ...` when using the decomposedfs, eg. with the oCIS storage. We now check if the link exists before trying to create it.

https://github.com/cs3org/reva/pull/1992