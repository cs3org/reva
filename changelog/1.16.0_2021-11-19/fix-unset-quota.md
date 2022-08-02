Bugfix: Fix unset quota xattr on darwin

Unset quota attributes were creating errors in the logfile on darwin.

https://github.com/cs3org/reva/pull/2260
