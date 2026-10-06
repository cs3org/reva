Bugfix: Reconnect the service registry when publishing fails

A process whose NATS write failed marked its registry connection as lost and
queued the write, but only the watch path ever reconnected. A process with a
healthy watcher therefore never reconnected: it kept queueing its own
heartbeats, silently, while its peers aged its nodes out and stopped being able
to resolve it, until the process was restarted. Writes now reconnect too, so a
NATS outage recovers on the next heartbeat, and a reconnect no longer leaves the
previous connection open.

https://github.com/cs3org/reva/pull/5817
