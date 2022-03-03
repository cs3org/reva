Enhancement: Don't assume that the LDAP groupid in reva matches the name

This allows using attributes like e.g. `entryUUID` or any custom id attribute
as the id for groups.

https://github.com/cs3org/reva/pull/2345
