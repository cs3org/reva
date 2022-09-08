Enhancement: Improve ldap authprovider's error reporting

The errorcode returned by the ldap authprovider driver is a bit more explicit
now. (i.e. we return a proper Invalid Credentials error now, when the LDAP Bind
operation fails with that)

https://github.com/cs3org/reva/pull/3185
