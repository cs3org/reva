Security: guard inbound share discovery against SSRF

Unauthenticated inbound `/shares` discovery now uses a public-only HTTP
client built once at handler init and reused per request, closing the SSRF
where a sender-derived server could steer the inbound service at private
networks. Loopback is allowed only behind an explicit
`allow_loopback_federation` flag for the local two-provider integration
topology; both local integration fixtures declare the exception. The
discovery timeout is now configurable via `ocm_client_timeout` (seconds,
default 10, preserving the existing inbound-discovery timeout). Operators
can opt the inbound public-only client into honoring `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY` via `ocm_client_use_env_proxy` (off by
default; the public-only client stays direct unless explicitly enabled).

Controlled federation deployments with private RFC 1918 / IPv6 ULA
destinations can opt the inbound public-only discovery client into a narrow
address exception via `allowed_federation_cidrs` (off by default; each entry
must be a canonical CIDR wholly inside `10.0.0.0/8`, `172.16.0.0/12`,
`192.168.0.0/16`, or `fc00::/7`). An invalid element fails service startup
before any discovery runs: no partially accepted list is installed and the
router is not served. Ordinary public destinations remain permitted, loopback
stays gated behind `allow_loopback_federation`, and unrelated private ranges
remain denied.

https://github.com/cs3org/reva/pull/5836
