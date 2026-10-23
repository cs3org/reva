Changelog for reva 3.14.0 (2026-10-23)
=======================================

The following sections list the changes in reva 3.14.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5892: Public links on cephmount
 * Fix #5774: Report an error when a COPY or third-party copy upload fails
 * Fix #5942: Take EOS sysmask into account when parsing modbits
 * Fix #5879: Do not enable token cache for public link tokens
 * Fix #5903: Fix apps on received OCM shares
 * Chg #5781: Return tailored grpc error codes instead of always INTERNAL
 * Enh #5914: Let a service act on the Admin API
 * Enh #5863: Expose admin status and impersonation over HTTP
 * Enh #5900: Delegate project permissions to the gateway
 * Enh #5869: Introduce rate limits on OCM endpoints
 * Enh #5902: Mutual TLS between gRPC services
 * Enh #5924: Run shared invocations on one instance
 * Enh #5944: Stream a stream-only invocation without -stream
 * Enh #5925: Typed invocation arguments
 * Enh #5827: Native Kerberos authentication
 * Enh #5867: Add option to limit space concurrency of reconc. jobs
 * Enh #5866: Implement job to remove ACLs of expired shares
 * Enh #5932: Reopen the log files on SIGUSR2
 * Enh #5873: Cap the concurrent runs of a background job
 * Enh #5874: Stop claiming background jobs on a drained node
 * Enh #5870: Enqueue background jobs from any process
 * Enh #5871: Report the progress of background jobs
 * Enh #5875: Prune old background job runs
 * Enh #5872: Let background jobs choose when to retry
 * Enh #5928: Show the logs of a job run
 * Enh #5912: Properly handle setting, updating, removal of share expiration
 * Enh #5898: Enrich EOS permissions with space-assigned roles
 * Enh #5792: Expose service statistics as Prometheus metrics
 * Enh #5943: Per-owner share and project statistics
 * Enh #5923: Wopi: make recent UI-oriented changes wired in the CS3APIs

Details
-------

 * Bugfix #5892: Public links on cephmount

   * Public links and OCM shares take their role from the token, as on EOS, instead of requiring a
   grant xattr that nothing ever wrote. * The cboxexternal service account now gets rwx wherever
   an external account goes, on first use, and keeps it. * External accounts cannot go through
   symlinks, and a stat never changes the ACL.

   https://github.com/cs3org/reva/pull/5892

 * Bugfix #5774: Report an error when a COPY or third-party copy upload fails

   Both reported success when the upload failed, leaving the destination unwritten, and a
   third-party copy push of a missing source panicked instead of returning 404.

   https://github.com/cs3org/reva/pull/5774

 * Bugfix #5942: Take EOS sysmask into account when parsing modbits

   https://github.com/cs3org/reva/pull/5942

 * Bugfix #5879: Do not enable token cache for public link tokens

   https://github.com/cs3org/reva/pull/5879

 * Bugfix #5903: Fix apps on received OCM shares

   Apps on received ocm shares were broken because the mime-type was set incorrectly and the path
   to the file that was given to wopi was incorrect

   https://github.com/cs3org/reva/pull/5903

 * Change #5781: Return tailored grpc error codes instead of always INTERNAL

   Many grpc handlers wrapped every error in status.NewInternal, hiding the real code from
   callers. They now map known errtypes (not found, permission denied, already exists, invalid
   argument, ...) to the matching grpc status code through a fixed
   status.NewStatusFromErrType, and the gateway share handlers propagate the downstream
   status instead of flattening it. The auth-related services keep the code but return a generic
   message through status.NewRedactedStatusFromErrType, so internal failures are only
   logged.

   https://github.com/cs3org/reva/issues/4915
   https://github.com/cs3org/reva/pull/5781

 * Enhancement #5914: Let a service act on the Admin API

   `admin.AsService` gives a reva service a short-lived token for the synthetic user
   `service:<name>`, with only the admin scope, so it can call the Admin API and appears under its
   own name in the audit log.

   https://github.com/cs3org/reva/pull/5914

 * Enhancement #5863: Expose admin status and impersonation over HTTP

   A new `admin` HTTP service lets clients that only speak the public HTTPS surface use the Admin
   API's user-facing part. `GET /admin/status` reports whether the caller is an admin, backed by
   a new `CheckAdmin` RPC that mints nothing and so is not audited. `POST /admin/impersonate`
   steps the caller up and impersonates the target in one request, so the admin token never leaves
   the server; the step-up and the impersonation are both audited as before, with a reason if the
   client gives one. Impersonation tokens no longer share the admin token's short lifetime: they
   last as long as a token from signing in, or `impersonation_ttl`, so a transfer made as the
   impersonated user can finish.

   https://github.com/cs3org/reva/pull/5863

 * Enhancement #5900: Delegate project permissions to the gateway

   Now ceph delegates the project permissions to share to the gateway, that obtains the
   information from the space registry, since there's no way to know in the cephmount driver.

   https://github.com/cs3org/reva/pull/5900

 * Enhancement #5869: Introduce rate limits on OCM endpoints

   This PR introduces rate-limiting for all OCM-related requests, answering 429 with
   Retry-After once a client is over its limit. The defaults are max 60 reqs/minute with 20 reqs per
   burst. The same limiter guards the unauthenticated routes of ocdav (public links, legacy
   links, status) and of appprovider (/list), with a default of 600 reqs/minute and 100 reqs per
   burst. The loginflow (apptoken) endpoints now use it too, which bounds its memory and adds the
   `ratelimit.trusted_proxy_cidrs` key. A single per-IP limiter now guards all its routes, the
   loginflow included, configured by `ratelimit.syncclient_per_ip_per_min`, which
   replaces the `ratelimit.init_per_ip_per_min` and `ratelimit.poll_per_ip_per_min`
   keys.

   https://github.com/cs3org/reva/pull/5869

 * Enhancement #5902: Mutual TLS between gRPC services

   With the service registry, reva dials backend services on other hosts, so their ports must be
   reachable, which also lets a user skip the gateway by calling a backend directly. The services
   can now enable mTLS for incoming and outgoing gRPC connections in order to avoid having users
   call the backend directly.

   https://github.com/cs3org/reva/pull/5902

 * Enhancement #5924: Run shared invocations on one instance

   Invocations that act on shared state, like the jobs store, can be marked `Shared()`. Admin then
   sends them to one live instance per service instead of to every instance, so `reva admin invoke
   jobs enqueue` no longer enqueues one run per runner.

   https://github.com/cs3org/reva/pull/5924

 * Enhancement #5944: Stream a stream-only invocation without -stream

   `reva admin invoke` now streams an invocation that has only a streaming form. The invocation
   spec gets `stream_only` for this, and `-stream` is still needed to follow an invocation with
   both forms, like `logs`.

   https://github.com/cs3org/reva/pull/5944

 * Enhancement #5925: Typed invocation arguments

   An invocation handler now takes its arguments as a struct with `arg` tags, made with
   `invoke.Typed`, and the framework parses them before the handler runs, so a bad value is
   rejected instead of read as zero. Fields can be strings, numbers, bools, durations, times
   (RFC3339 or a duration meaning that long ago), any `encoding.TextUnmarshaler`, or
   comma-separated slices of these.

   https://github.com/cs3org/reva/pull/5925

 * Enhancement #5827: Native Kerberos authentication

   Reva can now verify a Kerberos ticket itself, rather than relying on an identity provider to
   vouch for one.

   Two pieces make it up: a `kerberos` auth manager that verifies a SPNEGO token against a keytab
   and resolves the principal to a user, and a `spnego` credential strategy that reads
   `Authorization: Negotiate` from a request and challenges an unauthenticated one. No new
   endpoint comes with it — every authenticated response already carries the issued token in
   `x-access-token`, so a client presents its ticket to an endpoint it was going to call anyway.

   Only the ticket is trusted: any username the client also sends is ignored, so a valid ticket
   cannot be used to ask for someone else's session. The keytab is re-read when it changes on disk,
   so rotating it needs no restart.

   https://github.com/cs3org/reva/pull/5827

 * Enhancement #5867: Add option to limit space concurrency of reconc. jobs

   https://github.com/cs3org/reva/pull/5867

 * Enhancement #5866: Implement job to remove ACLs of expired shares

   https://github.com/cs3org/reva/pull/5866

 * Enhancement #5932: Reopen the log files on SIGUSR2

   Revad now reopens its log files when it receives SIGUSR2 (or `revad -s reopen`). Logs can then be
   rotated by renaming them, e.g. logrotate with `create` and a postrotate signal, instead of
   `copytruncate`, which drops the lines written while the copy is synced to disk and leaves log
   shippers that follow the file to notice the truncation.

   https://github.com/cs3org/reva/pull/5932

 * Enhancement #5873: Cap the concurrent runs of a background job

   `max_concurrent` gives a job its own workers, so it cannot take the full pool, and
   `periodic_reserve` keeps workers for leader periodic jobs. Claims rotate across jobs.

   https://github.com/cs3org/reva/pull/5873

 * Enhancement #5874: Stop claiming background jobs on a drained node

   Draining a node pauses its jobs runner, and running jobs that implement `HandOffJob` are asked
   to move to another node.

   https://github.com/cs3org/reva/pull/5874

 * Enhancement #5870: Enqueue background jobs from any process

   A process can enqueue an on-demand job registered by another runner, and `reva admin jobs
   forget` drops a removed job.

   https://github.com/cs3org/reva/pull/5870

 * Enhancement #5871: Report the progress of background jobs

   A job can report how far it has got while it runs; the progress is stored with the run and shown by
   `reva admin jobs`.

   https://github.com/cs3org/reva/pull/5871

 * Enhancement #5875: Prune old background job runs

   The `rjobs.retention` job deletes finished runs older than `run_retention_days` (30 by
   default) from the status DB.

   https://github.com/cs3org/reva/pull/5875

 * Enhancement #5872: Let background jobs choose when to retry

   A job can ask to be retried after a given delay, or give up with a permanent error that ends the run
   as `aborted`. The attempt number now counts deliveries.

   https://github.com/cs3org/reva/pull/5872

 * Enhancement #5928: Show the logs of a job run

   A job run uses its run id as trace id and passes its job name to the services it calls, which log it as
   `job`. `reva admin jobs logs <run-id>` shows the lines of one run from every instance, `-job
   <name>` those of every run of a job, and `-f` follows them.

   https://github.com/cs3org/reva/pull/5928

 * Enhancement #5912: Properly handle setting, updating, removal of share expiration

   https://github.com/cs3org/reva/pull/5912

 * Enhancement #5898: Enrich EOS permissions with space-assigned roles

   https://github.com/cs3org/reva/pull/5898

 * Enhancement #5792: Expose service statistics as Prometheus metrics

   Services can now publish statistics (share and public-link counts by status, permission
   class, item type and storage instance; creator and recipient counts) on reva's Prometheus
   endpoint.

   A driver opts in by implementing the `stats.Reporter` capability (`pkg/stats`), which its
   service exposes as a `stats` invocation — the same numbers are then available to operators
   via `reva admin invoke <service> stats`. A new zero-configuration collector served by the
   existing `prometheus` HTTP service discovers the services advertising the invocation
   through the service registry, queries them over the control channel on a background refresh
   loop, and serves cached metrics. The SQL share and public-link managers implement the
   capability with in-driver aggregate queries.

   Payloads are self-describing (names, kinds, labels), so new services need no collector
   changes. Per-owner aggregates can be enriched with site-defined owner attributes from a JSON
   file (`/etc/revad/owner-attributes.json` by convention): the attribute keys become
   metric labels, and reva itself stays agnostic of their meaning. The invocation fan-out
   machinery moves from the admin service to the shared `pkg/invoke/client`.

   https://github.com/cs3org/reva/pull/5792

 * Enhancement #5943: Per-owner share and project statistics

   The share managers count active shares per creator and the spaces registry reports projects,
   quota and used space per owner, so the stats collector can sum them by owner attributes.
   `owner_attributes` and `owner_attributes_key` choose the labels and where the owner map is
   in the file. The collector now finds and streams the stats through the Admin API.

   https://github.com/cs3org/reva/pull/5943

 * Enhancement #5923: Wopi: make recent UI-oriented changes wired in the CS3APIs

   https://github.com/cs3org/reva/pull/5923


