Enhancement: Expose service statistics as Prometheus metrics

Services can now publish statistics (share and public-link counts by
status, permission class, item type and storage instance; creation
counters; creator and recipient counts) on reva's Prometheus endpoint.

A driver opts in by implementing the `stats.Reporter` capability
(`pkg/stats`), which its service exposes as a `stats` invocation — the
same numbers are then available to operators via
`reva admin invoke <service> stats`. A new zero-configuration collector
served by the existing `prometheus` HTTP service discovers the services
advertising the invocation through the service registry, queries them
over the control channel on a background refresh loop, and serves cached
metrics. The SQL share and public-link managers implement the capability
with in-driver aggregate queries.

Payloads are self-describing (names, kinds, labels), so new services need
no collector changes. Per-owner aggregates can be enriched with
site-defined owner attributes from a JSON file
(`/etc/revad/owner-attributes.json` by convention): the attribute keys
become metric labels, and reva itself stays agnostic of their meaning.
The invocation fan-out machinery moves from the admin service to the
shared `pkg/invoke/client`.

https://github.com/cs3org/reva/pull/5792
