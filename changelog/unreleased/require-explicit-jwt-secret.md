Security: require an explicit shared JWT secret

Reva no longer falls back to a built-in JWT signing key. Deployments relying on
the shared secret must configure `shared.jwt_secret` with a high-entropy value
and share it only among the Reva services in the same deployment. If neither a
shared nor token-manager-specific secret is configured, JWT manager creation
now fails with an actionable error. Missing configuration can no longer select a
publicly known signing key.
