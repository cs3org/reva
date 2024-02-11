Bugfix: Don't return disabled users in GetUser call

We fixed a bug where it was still possible to lookup a disabled User if
the user's ID was known.

https://github.com/cs3org/reva/pull/4427
https://github.com/cs3org/reva/pull/4426
https://github.com/owncloud/ocis/issues/7962
