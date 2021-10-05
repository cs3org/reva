Enhancement: Don't assume that the LDAP userid in reva matches the user's user

This allows using attributes like e.g. `entryUUID` or any custom id attribute
as the id for users.

https://github.com/cs3org/reva/pull/2133
