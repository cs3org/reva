Enhancement: gate outbound webapp offers and preserve configured names

Outbound OCM share creation now offers webapp access only when the provider is
explicitly enabled, the request carries a webapp candidate, the remote
resource advertises webapp-receive with the blank target, and the remote
provider advertises exchange-token. The configured application name and
complete opener endpoint are used consistently in the outgoing request and
the persisted share. Offered webapp requirements are normalized before the
share is wired or stored: empty requirements default to must-exchange-token,
supplied requirements must be known, non-blank, unpadded, and unique, and
mixed webapp and WebDAV requirements must agree as sets. Invalid enabled
configuration fails startup, and normalization errors return a safe
invalid-argument error with no remote POST or store call.
