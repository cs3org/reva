Enhancement: OCM code-flow token exchange

Added end-to-end OCM code-flow support for both sender and receiver paths. Shares
can now declare `requirements: ["must-exchange-token"]` and
`accessTypes: ["remote"]`, with the new fields preserved across both SQL and JSON
persistence. Discovery advertises `tokenEndPoint` and `exchange-token`, the new
`POST /ocm/token` endpoint exchanges authorization codes into short-lived JWTs,
and the dedicated `ocmsharecode` and `ocmexchangedtoken` auth managers separate
exchange-code validation from exchanged-token validation.

The same feature branch also hardens the runtime path uncovered during live
interop validation. Code-flow scopes now carry share and resource identity
without embedding the long-lived shared secret, malformed protocol payloads are
rejected earlier, and the validated interop fixes stay in the same change set:
correct `client_id` handling, root-mounted DAV share recovery for
Nextcloud-style clients, preserved download paths for root-mounted single-file
reads, and the related WOPI external-link fix that prefers canonical share ids
over legacy tokens.

Validation coverage was expanded at the seams that changed most. The branch now
includes focused tests for `/ocm/token` behavior, discovery-to-route coupling,
WOPI share-id fallback, received-side token-endpoint discovery and exchange
helpers, received-side retry wrappers, and persistence/validation behavior.
Legacy direct-secret flows remain operational, while code-flow shares enforce
token exchange as an explicit protocol requirement.

https://github.com/cs3org/reva/pull/5552
