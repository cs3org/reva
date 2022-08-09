Bugfix: Fix crash in ldap authprovider

We fixed possible crash in the LDAP authprovider caused by a null pointer
derefence, when the IDP settings of the userprovider are different from
the authprovider.

https://github.com/cs3org/reva/pull/3086
