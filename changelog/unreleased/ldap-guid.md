Enhancement: Allow using AD UUID as userId values

Active Directory UUID attributes (like e.g. objectGUID) use the LDAP octectString
Syntax. In order to be able to use them as userids in reva, they need to be converted
to their string representation.

https://github.com/cs3org/reva/pull/2525
