Bugfix: Report registry write failures and stop serving with an unresolvable peer

A failed write to the NATS service registry was swallowed. The driver latched an
internal "disconnected" flag on the first failed put, queued every later write,
and returned success, and only a new watch could clear the flag. A single slow
JetStream ack was therefore enough to make a healthy process disappear from the
registry for the rest of its life, silently: it kept serving while its peers
could no longer resolve it, and its own log said nothing.

Registry writes now report what did not land, and the driver reads its
connection state from the connection instead of a cached flag, so a failed write
is retried on the next heartbeat and a closed connection is redialled.

Peer resolution also no longer fails a request on the first miss. A lookup is
retried, which covers a peer that is starting or whose registration has not
propagated yet. A peer that stays unresolvable for about a minute now ends the
process instead of logging an error per request, so an instance that cannot
route is visibly dead rather than quietly broken.
