Enhancement: cap the concurrent runs of a background job

`max_concurrent` gives a job its own workers, so it cannot take the full pool, and `periodic_reserve` keeps workers for leader periodic jobs. Claims rotate across jobs.

https://github.com/cs3org/reva/pull/5873
