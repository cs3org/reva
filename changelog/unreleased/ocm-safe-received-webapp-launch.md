Enhancement: add safe received webapp launch

ScienceMesh `OpenInApp` now launches received OCM webapp shares. The stored
offer is validated before any dial (`must-exchange-token`, `blank` target,
https app URI), then the sender is discovered and the shared secret is
exchanged for an access token over the public-only OCM client, with the
configured provider domain as `client_id`. The response is `app_url` and
`access_token`; the secret stays on the server. `must-use-mfa` offers are
refused unless `mfa_policy` in `[http.services.wellknown.ocmprovider]` is
`off` (default `reject`); nothing is advertised in discovery. The launch uses
the ScienceMesh timeout, TLS settings and explicit `allowed_federation_cidrs`
for both discovery and token exchange. Without an exception, private
addresses are refused; loopback and environment proxies remain disabled on
this service.

https://github.com/cs3org/reva/pull/5850
