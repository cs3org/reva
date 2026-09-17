Enhancement: Native Kerberos authentication

Reva can now verify a Kerberos ticket itself, rather than relying on an identity
provider to vouch for one.

Two pieces make it up: a `kerberos` auth manager that verifies a SPNEGO token
against a keytab and resolves the principal to a user, and a `spnego` credential
strategy that reads `Authorization: Negotiate` from a request and challenges an
unauthenticated one. No new endpoint comes with it — every authenticated response
already carries the issued token in `x-access-token`, so a client presents its
ticket to an endpoint it was going to call anyway.

Only the ticket is trusted: any username the client also sends is ignored, so a
valid ticket cannot be used to ask for someone else's session. The keytab is
re-read when it changes on disk, so rotating it needs no restart.

https://github.com/cs3org/reva/pull/5827
