Security: extend OCM untrusted HTTP client hardening

Outbound OCM HTTP clients for peer-supplied URLs now share the additive
UntrustedClientSecurity type and a public-only transport. net.Dialer.Control
refuses non-public dial targets against SSRF, DNS rebinding, and cloud
metadata access; URLs must be HTTPS; redirect chains default to three hops;
OCM JSON response bodies default to 1 MiB; and TLS defaults to 1.2.
The ocm_client_security block and ocm_client_response_limit,
ocm_client_tls_min_version, ocm_client_dial_timeout, and ocm_client_timeout
knobs remain configurable. Received storage split WebDAV metadata clients from
WebDAV streaming clients, with webdav_dial_timeout for dial deadlines and
webdav_timeout for metadata request deadlines (streams stay uncapped);
webdav_transfer_timeout is reserved for a follow-up idle stall monitor and
does not cap streams. ocm_allow_loopback_federation adds
an off-by-default, WebDAV-only loopback hatch. ScienceMesh reuses the ocmd
public-only client, regression tests cover the hardening paths, and unused
gateway WebDAV storage provider code was removed.

https://github.com/cs3org/reva/pull/5389
