
---
title: "v3.10.3"
linkTitle: "v3.10.3"
weight: 999597
description: >
  Changelog for Reva v3.10.3 (2026-06-30)
---

Changelog for reva 3.10.3 (2026-06-30)
=======================================

The following sections list the changes in reva 3.10.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5683: Redact machine auth secret from log
 * Fix #5684: Ignore basic auth if a signature is present
 * Enh #5677: Cancel background jobs and trigger scheduled jobs on demand
 * Enh #5672: Associate on-demand jobs with a user
 * Enh #5682: Configure on-demand jobs from the config file

Details
-------

 * Bugfix #5683: Redact machine auth secret from log

   https://github.com/cs3org/reva/pull/5683

 * Bugfix #5684: Ignore basic auth if a signature is present

   https://github.com/cs3org/reva/pull/5684

 * Enhancement #5677: Cancel background jobs and trigger scheduled jobs on demand

   Added the ability to cancel a background job run, both on-demand and leader-scoped periodic,
   and to trigger a scheduled job to run immediately. Cancellation is cooperative through the
   job's context, durable so it reaches a run that is still queued or running on another process,
   and terminal so a cancelled run is not retried. Triggering enqueues an extra run without
   disturbing the schedule's regular cadence.

   https://github.com/cs3org/reva/pull/5677

 * Enhancement #5672: Associate on-demand jobs with a user

   On-demand background jobs can now be attached to a user through a WithOwner enqueue option, so
   the jobs a user created can be listed back (e.g. for a UI) with ListByOwner; jobs enqueued
   without it stay internal. An opt-in Unique option was also added to keep at most one active run
   per owner and key, so a user cannot, for instance, start the same export twice.

   https://github.com/cs3org/reva/pull/5672

 * Enhancement #5682: Configure on-demand jobs from the config file

   On-demand background jobs are now given their own configuration section from the config file,
   handed to the job constructor the same way the other services receive their configuration.
   Each job reads its settings from [serverless.services.jobs.on_demand."<name>"], so it can
   load what it needs at startup instead of seeing only the per-run parameters.

   https://github.com/cs3org/reva/pull/5682


