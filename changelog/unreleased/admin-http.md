Enhancement: Expose admin status and impersonation over HTTP

A new `admin` HTTP service lets clients that only speak the public HTTPS
surface use the Admin API's user-facing part. `GET /admin/status` reports
whether the caller is an admin, backed by a new `CheckAdmin` RPC that mints
nothing and so is not audited. `POST /admin/impersonate` steps the caller up
and impersonates the target in one request, so the admin token never leaves the
server; the step-up and the impersonation are both audited as before, with a
reason if the client gives one. Impersonation tokens no longer share the admin
token's short lifetime: they last as long as a token from signing in, or
`impersonation_ttl`, so a transfer made as the impersonated user can finish.

https://github.com/cs3org/reva/pull/TODO
