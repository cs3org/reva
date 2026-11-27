Enhancement: read the fleet's log counts from the stats collector

Every service exposes a built-in `logcounts` invocation with its process's
counts of log messages at warn level or above. The prometheus `stats`
collector reads it across the fleet (every `log_counts_interval`, default
1m) and exports `<namespace>_fleet_log_messages_total{service, host, level}`,
named after each process's config file, so the services need no `/metrics`
endpoint of their own. The processes of a host sharing a config file name are
summed into one series, which stays monotonic when a process misses a refresh
or restarts. Zero or negative intervals are now rejected.

https://github.com/cs3org/reva/pull/6062
