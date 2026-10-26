Enhancement: make impersonation safe and visible in the audit

A token from `Impersonate` carries the impersonator in an `actor` scope,
which authorizes nothing and survives the auth cache and token re-mints.
The request logs and the audit events show it as `act`. `RequestAdmin` and
`Impersonate` refuse such a token, so an impersonated admin cannot step up.
Members of `admin_group` and service accounts need `allow_privileged`, which
is audited on its own line. A lightweight target gets the scopes of its own
login. The response has `expires_at`, the request takes an optional
`ttl_seconds` capped by the server, and a missing target answers NotFound.
The machine scope no longer satisfies admin requests.

https://github.com/cs3org/reva/pull/XXXX
