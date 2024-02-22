Bugfix: Fix truncating existing files

We fixed a problem where existing files kept their content when being overwritten by a 0-byte file.

https://github.com/cs3org/reva/pull/4448
