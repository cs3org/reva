Bugfix: complete code-flow OCM token client identity

The token endpoint accepts only the authorization_code grant. The form
client_id must be the receiving provider's host-only FQDN and must
match the stored share recipient. The minted JWT client_id claim is
the resolved outgoing share opaque id. Legacy direct-secret tokens
omit that claim. Missing, unknown, and ocm_share grants are rejected
before any share lookup.
