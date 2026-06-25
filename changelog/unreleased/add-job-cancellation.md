Enhancement: Cancel background jobs and trigger scheduled jobs on demand

Added the ability to cancel a background job run, both on-demand and
leader-scoped periodic, and to trigger a scheduled job to run immediately.
Cancellation is cooperative through the job's context, durable so it reaches a
run that is still queued or running on another process, and terminal so a
cancelled run is not retried. Triggering enqueues an extra run without
disturbing the schedule's regular cadence.

https://github.com/cs3org/reva/pull/5677
