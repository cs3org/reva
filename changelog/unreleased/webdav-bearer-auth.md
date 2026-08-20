Enhancement: allow WebDAV access with OIDC bearer tokens

The /webdav and /remote.php/webdav endpoints now accept
"Authorization: Bearer <OIDC access token>" in addition to basic auth,
by routing the bearer auth type to an authprovider running the oidc
auth manager. The bearer credential and token strategies now only match
a proper RFC 6750 Bearer scheme (case-insensitive) instead of treating
any Authorization header value as a bearer token, the /webdav path was
added to the share and lightweight scope whitelists for parity with
/remote.php/webdav, and the oidc auth manager gained an optional
`audience` config option to verify the aud claim of incoming tokens.

https://github.com/cs3org/reva/pull/5772
