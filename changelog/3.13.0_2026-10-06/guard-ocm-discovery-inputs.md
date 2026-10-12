Security: guard remaining OCM discovery inputs

The open provider authorizer's GetInfoByDomain and the ScienceMesh
directory-listed provider discovery now use the shared public-only OCM
client. The operator-configured directory fetch stays on the trusted
client; only provider URLs taken from the unsigned directory response
(and request-supplied WAYF discovery) go through the public-only client.
Loopback is disabled for open-authorizer discovery. Both untrusted paths
bypass environment proxies. Blocked targets are rejected before any request
reaches the server.

Operators may now configure an explicit `allowed_federation_cidrs` exception
list for the public-only OCM discovery clients (ScienceMesh WAYF and the open
provider authorizer). The list is empty by default and admits only canonical
RFC 1918 IPv4 or IPv6 ULA prefixes; invalid entries abort initialization
before any directory fetch. Directory fetches stay trusted; listed providers
and request-supplied discovery remain public-only with the configured
exception only. TLS verification, loopback, and proxy controls stay
independent.

https://github.com/cs3org/reva/pull/5838
