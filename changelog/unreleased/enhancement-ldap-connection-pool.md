Enhancement: Add an opt-in LDAP connection pool

Added `GetLDAPClientWithPool`, a bounded pool of authenticated LDAP
connections, as a drop-in alternative to the existing single, long-lived
reconnecting connection. It is disabled by default and can be enabled per
backend via `pool_enabled` (plus `pool_size` and `pool_checkout_timeout`) on
the auth, user and group LDAP managers, so concurrent requests no longer
serialize on a single socket.

https://github.com/owncloud/reva/pull/681
