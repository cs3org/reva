Bugfix: put the share id in the outbound OCM token

Code-flow token exchange now sets the JWT client_id claim to the
outgoing share opaque id. That id is the wire providerId, and it
comes from the share resolved for the exchanged code. The OAuth
request client_id, the server domain, and legacy direct-secret
tokens do not provide this claim.
