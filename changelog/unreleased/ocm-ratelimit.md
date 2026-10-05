Enhancement: rate limit unauthenticated OCM endpoints

The OCM routes that bypass HTTP authentication are now rate limited per
client IP (IPv6 per /64) with a token bucket, answering `429` with
`Retry-After` once a client is over its limit. This covers the `ocm` ingress
routes (`/shares`, `/invite-accepted`, `/notifications`, `/token`),
`/.well-known/ocm`, and the ScienceMesh WAYF routes `/federations` and
`/discover`. Each service accepts `unauth_rate_limit` (requests per minute,
default 60, negative to disable), `unauth_rate_limit_burst` (default 20),
and `trusted_proxy_cidrs`, the proxies whose `X-Forwarded-For` header
identifies the client. An invalid `trusted_proxy_cidrs` entry aborts service
startup.

https://github.com/cs3org/reva/pull/TODO
