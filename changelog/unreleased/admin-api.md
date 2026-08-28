Enhancement: Add the Reva Admin API

Reva gains an operator API for inspecting and safely operating a running
deployment, driven by a new `reva admin` command. It is authenticated, audited,
and off by default — it turns on only where a process is configured with an
admin group.

At its core is a lightweight control channel that every reva process runs next
to its normal services. Through it the admin can reach a live service instance to
read its state or ask it to perform an operation (an "invocation"). Targets are
resolved through the service registry, so a single command can address one
instance, every instance of a service, or the whole fleet, and the results come
back merged.

With this an operator can see what the fleet is doing — list services and their
health, read and diff configuration, read or follow logs, trace a single request
or user across services, dump goroutines, and watch an instance's in-flight
request activity — and act on it — set a process's log level at runtime, take
instances out of and back into rotation for maintenance, drive the background
jobs runner (list and enqueue jobs, trigger or cancel runs), and impersonate a
user.

On the same host it needs no login: it is reached over a Unix socket and
authorised by the caller's OS identity. Remotely, an operator first steps up to a
short-lived, admin-only token.

https://github.com/cs3org/reva/pull/5692
