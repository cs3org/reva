Security: guard inbound share discovery against SSRF

Unauthenticated inbound `/shares` discovery now uses a public-only HTTP
client built once at handler init and reused per request, closing the SSRF
where a sender-derived server could steer the inbound service at private
networks. Loopback is allowed only behind an explicit
`allow_loopback_federation` flag for the local two-provider integration
topology; both local integration fixtures declare the exception. The
discovery timeout is now configurable via `ocm_client_timeout` (seconds,
default 10, preserving the existing inbound-discovery timeout).

https://github.com/cs3org/reva/pull/5836
