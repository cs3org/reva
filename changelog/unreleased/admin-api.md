Enhancement: Add the Reva Admin API

Reva gains an authenticated operator API for inspecting a running deployment and
performing bounded, audited actions on it. After stepping up to a short-lived
admin token, an operator can list the fleet's services with their nodes and
health, read a service's effective (redacted) configuration, run operations a
service chooses to expose, read or follow a service's recent logs, trace one
request or user across the fleet, adjust a process's log level at runtime, and
impersonate a user. Services expose those operations over a small control channel
that each process runs, and a new `reva admin` command drives the whole surface. On the same host it works with no
login at all — reached over a Unix socket and authenticated by the caller's OS
identity. It stays disabled unless a process is configured with an admin group.

https://github.com/cs3org/reva/pull/5692
