Enhancement: Add the Reva Admin API

Reva gains an authenticated operator API for inspecting a running deployment and
performing bounded, audited actions on it. After stepping up to a short-lived
admin token, an operator can list the fleet's services with their nodes and
health, read a service's effective (redacted) configuration, run operations a
service chooses to expose, and impersonate a user. Services expose those
operations over a small control channel that each process runs, and a new
`reva admin` command drives the whole surface. It stays disabled unless a
process is configured with an admin group.

https://github.com/cs3org/reva/pull/5692
