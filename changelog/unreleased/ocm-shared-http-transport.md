Enhancement: add shared OCM HTTP transport package

Introduces `pkg/ocm/client` with `TransportConfig` and trusted/public-only
HTTP client constructors, centralizing the transport and address-policy
logic previously inlined in `ocmd`. Public-only clients keep bypassing
environment proxies and using the guarded dialer; existing
`ocmd.OCMClient` callers are unchanged.

https://github.com/cs3org/reva/pull/5833
