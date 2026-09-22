Enhancement: add shared OCM HTTP transport package

Introduces `pkg/ocm/client` with `TransportConfig` and trusted/public-only
HTTP client constructors, centralizing the transport and address-policy
logic previously inlined in `ocmd`. The public-only dial guard refuses
non-public targets and rejects IPv6 addresses carrying a zone suffix, which
would otherwise bypass the NAT64 and denied-prefix checks. Public-only
clients default to direct mode and ignore environment proxies; a service can
opt in to honoring `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` via the new
`UseEnvProxy` transport field. Trusted clients honor environment proxies
regardless. Existing `ocmd.OCMClient` callers are unchanged.

https://github.com/cs3org/reva/pull/5833
