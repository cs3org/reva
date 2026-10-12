Enhancement: Fail over to another node when a peer cannot be reached

A client resolved through the service registry used to be pinned to the node it
resolved at, so a call to a peer that had died failed even when other nodes of
that service were serving. The caller had to know about nodes to do anything
about it.

Clients now resolve a node per call. A call that fails because its node could not
be reached is sent to another node of the same service, up to three distinct
nodes, within the caller's deadline. Whether a call may be replayed is decided
from what the connection was doing when it broke: a connection that was not yet
ready never handed the request over, so any call is moved on, while a connection
that broke mid-flight only replays reads, never a write.

A node that could not be reached is then passed over for thirty seconds, so the
calls that follow start somewhere else instead of rediscovering the same dead
peer, and is picked up again as soon as it answers. This is tracked per process,
because a node re-registers itself as ready on every heartbeat: the failure worth
reacting to — a peer that is healthy to the registry but unreachable from here —
is one only the calling process can observe.

None of this is visible to the caller, which still asks for a client and makes a
call.

https://github.com/cs3org/reva/pull/5841
