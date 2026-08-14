Security: harden OCM outbound HTTP clients after public-only client

Following the public-only client work merged in #5752, outbound OCM HTTP
calls now share tighter limits and one untrusted transport path. Discovery,
share, and ExchangeToken response bodies are capped at 1 MiB with
io.LimitReader. NewPublicOnlyClient enforces HTTPS-only URLs, limits
redirects to three hops, and sets TLS 1.2 as the minimum on OCM client
transports.

ScienceMesh gained flat ocm_client_response_limit and ocm_client_timeout
config fields. The unauthenticated /discover (WAYF) flow uses the untrusted
public-only client. Body-supplied discovery in the open authorizer and on
received Discover/ExchangeToken requests is routed through NewPublicOnlyClient,
closing a redirect-to-link-local SSRF on received Discover.

An exported UntrustedHTTPTransport helper lets gowebdav callers use the same
dial rules. That transport is installed on WebDAV ingest-stat, received-share
WebDAV access, and embedded srcURL fetch. A cohesive public-only dial-refusal
test matrix covers the refusal cases.

https://github.com/cs3org/reva/pull/5389
https://github.com/cs3org/reva/pull/5752
