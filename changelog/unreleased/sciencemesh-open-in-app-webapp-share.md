Enhancement: Add webapp launch support to sciencemesh OpenInApp

This adds the receiver-side launch flow for OCM webapp shares in
`/sciencemesh/open-in-app`.

When a received share carries the `webapp` protocol with
`must-exchange-token`, Reva now resolves the sender origin from the share
protocols, discovers the sender `tokenEndPoint`, exchanges the shared secret for
an access token, and returns a launch payload containing both `app_url` and
`access_token`.

The handler also validates that the received share actually contains a usable
webapp protocol (`uri`, `sharedSecret`, and `must-exchange-token`) and maps
discovery and token-exchange failures to API errors that better match the
receiver-side failure mode.

This lets a Reva receiver launch OCM webapp shares through the spec-defined
code-flow path instead of only returning a templated URL.

https://github.com/cs3org/reva/pull/5701
