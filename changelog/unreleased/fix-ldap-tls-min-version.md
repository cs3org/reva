Bugfix: Restore TLS 1.2 floor for LDAP connections

`tlsConfigFromLDAPConn` (shared by `GetLDAPClientWithReconnect`, `GetLDAPClientWithPool` and
`GetLDAPClientForAuth`) built its `*tls.Config` without a `MinVersion`, allowing the LDAP client
to negotiate down to whatever the Go runtime's default minimum TLS version is. Set
`MinVersion: tls.VersionTLS12` on both the insecure and CA-cert paths to match the floor the
callers previously enforced individually.

https://github.com/owncloud/reva/pull/681
