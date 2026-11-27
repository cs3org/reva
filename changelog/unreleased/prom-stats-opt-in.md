Enhancement: start the stats collector only when configured

The fleet stats collector of the prometheus service now starts only when
`[http.services.prometheus.stats]` is present. Any revad can then serve
/metrics, e.g. for reva_log_messages_total, without querying the fleet
through the Admin API every few minutes. Sites that relied on the
implicit default must add an (even empty) stats section to keep the
stats metrics.

https://github.com/cs3org/reva/pull/TBD
