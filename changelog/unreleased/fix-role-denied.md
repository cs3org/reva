Bugfix: add support for Deny Role

In OCS, we had a `RoleDenied`, which denied all permissions to a user. We now also ported
this to libregraph.

https://github.com/cs3org/reva/pull/5366
