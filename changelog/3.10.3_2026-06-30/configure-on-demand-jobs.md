Enhancement: configure on-demand jobs from the config file

On-demand background jobs are now given their own configuration section from
the config file, handed to the job constructor the same way the other services
receive their configuration. Each job reads its settings from
[serverless.services.jobs.on_demand."<name>"], so it can load what it needs at
startup instead of seeing only the per-run parameters.

https://github.com/cs3org/reva/pull/5682
