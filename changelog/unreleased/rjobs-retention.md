Enhancement: prune old background job runs

The `rjobs.retention` job deletes finished runs older than `run_retention_days` (30 by default) from the status DB.

https://github.com/cs3org/reva/pull/5875
