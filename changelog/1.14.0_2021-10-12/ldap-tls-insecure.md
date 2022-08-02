Enhancement: Safer defaults for TLS verification on LDAP connections

The LDAP client connections were hardcoded to ignore certificate validation
errors. Now verification is enabled by default and a new config parameter 'insecure'
is introduced to override that default. It is also possible to add trusted Certificates
by using the new 'cacert' config paramter.

https://github.com/cs3org/reva/pull/2053
