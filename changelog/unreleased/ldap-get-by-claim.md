Enhancement: Update LDAP user driver

The LDAP user driver can now fetch users by a single claim / attribute. Use an `attributefilter` like `(&(objectclass=posixAccount)({{attr}}={{value}}))` in the driver section.

It also adds the uid and gid to the users opaque properties so that eos can use them for chown and acl operations.

https://github.com/cs3org/reva/pull/1088