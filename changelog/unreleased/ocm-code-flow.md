Enhancement: OCM code-flow token exchange (Milestone M6)

Added spec-strict OCM code-flow support for both sender and receiver paths.
Shares can now carry `requirements: ["must-exchange-token"]` and `accessTypes: ["remote"]`
which are persisted immutably in both SQL and JSON repositories. The sender advertises
a `tokenEndPoint` and `exchange-token` capability via discovery, exposes a `POST /ocm/token`
endpoint for authorization_code exchange, and routes incoming DAV requests by shareId with
JWT-based auth. The receiver discovers the token endpoint, exchanges the shared secret for
a short-lived JWT before every remote WebDAV operation, and retries once on 401. Two new
auth managers (`ocmsharecode` and `ocmexchangedtoken`) handle the exchange and validation
of code-flow credentials. Legacy direct-secret flows remain fully operational.
