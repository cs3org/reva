Enhancement: read the fleet's log counts from the stats collector

Every service exposes a built-in `logcounts` invocation with its process's
counts of log messages at warn level or above. The prometheus `stats`
collector reads it across the fleet (every `log_counts_interval`, default
1m) and exports `<namespace>_fleet_log_messages_total{service, host, level}`,
one series per process named after its config file, so the services need no
`/metrics` endpoint of their own.

https://github.com/cs3org/reva/pull/TBD
