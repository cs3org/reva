Bugfix: fix data race in the OIDC provider cache

Concurrent logins for an issuer that was not cached yet could take revad down
with Go's unrecoverable "concurrent map writes".

https://github.com/cs3org/reva/pull/5765
