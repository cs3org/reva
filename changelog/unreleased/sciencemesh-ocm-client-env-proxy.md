Enhancement: opt ScienceMesh clients into the env proxy

ScienceMesh WAYF listed-provider discovery, request-supplied `/discover`,
and open-in-app discovery and token exchange honor `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY` when `ocm_client_use_env_proxy` is set on
`[http.services.sciencemesh]` (default false). The open provider
authorizer accepts the same key on
`[grpc.services.ocmproviderauthorizer.drivers.open]`. With the knob
enabled, the guard classifies the proxy hop: an unlisted private proxy
address is refused, and the CONNECT target is no longer separately
classified. Direct mode (knob off) preserves target classification.

https://github.com/cs3org/reva/pull/5850
