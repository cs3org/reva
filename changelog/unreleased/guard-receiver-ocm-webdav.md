Security: guard receiver OCM and WebDAV connections

Inbound legacy WebDAV type probes, received-share OCM discovery and token
exchange, and received-share WebDAV operations now all use the public-only
transport from the shared OCM client package. One stored OCM client guards
both discovery and token exchange; one driver helper builds every gowebdav
client and always applies the guarded transport, including the retry and
one-off upload paths. Loopback is allowed only behind an explicit
`allow_loopback_federation` flag on the received-storage config; private
targets stay denied. Enforcement is at dial time, not in URI syntax
validation.

https://github.com/cs3org/reva/pull/5837
