Change: decomposedfs no longer stores the idp

We no longer persist the IDP of a user id in decomposedfs grants. As a consequence listing or reading Grants no longer returns the IDP for the Creator.
It never did for the Grantee. Whatever credentials are used to authenticate a user we internally have to create a UUID anyway. Either by lookung it up in an external service (eg. LDAP or SIEM) or we autoprovision it.

https://github.com/cs3org/reva/pull/3267
