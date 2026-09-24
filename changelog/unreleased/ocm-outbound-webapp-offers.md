Enhancement: gate outbound webapp offers and preserve configured names

Outbound OCM share creation now offers webapp access only when the provider is
explicitly enabled and the remote provider supports webapp receiving and token
exchange. The configured application name and complete opener endpoint are
used consistently in the outgoing request and persisted share.
