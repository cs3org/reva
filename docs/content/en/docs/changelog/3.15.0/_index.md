
---
title: "v3.15.0"
linkTitle: "v3.15.0"
weight: 999550
description: >
  Changelog for Reva v3.15.0 (2026-10-28)
---

Changelog for reva 3.15.0 (2026-10-28)
=======================================

The following sections list the changes in reva 3.15.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5964: Give each cephmount thread request its own response channel
 * Fix #5946: Remove a data race when `revad` starts multiple configs
 * Chg #5842: Declare HTTP routes, and route between services with a gateway
 * Enh #5951: Add help-<target> to the Makefile
 * Enh #5950: Add a help target to the Makefile
 * Enh #5938: Report the progress of the expired shares job
 * Enh #5937: Report the progress of the orphan reconciliation
 * Enh #5936: Report the progress of the shallow reconciliation
 * Enh #5947: Rewrite the reva CLI on cobra
 * Enh #5948: Color the reva CLI help
 * Enh #5934: Let periodic jobs report progress
 * Enh #5940: Report the progress of the job retention
 * Enh #5939: Report the progress of the takeout cleanup
 * Enh #5935: Report the progress of takeouts

Details
-------

 * Bugfix #5964: Give each cephmount thread request its own response channel

   All requests of a cephmount user thread shared one response channel. When a request was
   cancelled, its response stayed in the channel, and the next request of that user read it and
   failed with "input/output error". Every later request of that user failed the same way, until
   the user was idle for 5 minutes. Parallel requests of one user could also read each other's
   responses. Each request now has its own response channel.

   https://github.com/cs3org/reva/pull/5964

 * Bugfix #5946: Remove a data race when `revad` starts multiple configs

   When `revad` started several configs, each instance registered itself in a package level
   slice from its own goroutine. Append is not thread-safe, so the slice could become corrupted.
   Nothing ever read the slice, so it was removed.

   https://github.com/cs3org/reva/pull/5946

 * Change #5842: Declare HTTP routes, and route between services with a gateway

   HTTP services used to hand the server one handler mounted under a configurable prefix, and
   route inside it however they liked. They now declare their routes on a router built on the
   standard library's ServeMux, and the server serves all of them from one table: patterns are
   absolute, methods are part of the route, and the paths exempt from authentication are declared
   on the routes themselves rather than kept in a separate list that could drift.

   Each service advertises its routes in the service registry, which is what the new `gateway`
   HTTP service mirrors: a single entry point that forwards a request to the service owning the
   route it matches, configured with no paths at all. It resolves the serving node per request and
   forwards the path exactly as it arrived, as WebDAV clients require, leaving authentication to
   the service.

   Two consequences for existing configurations. The `prefix` setting of every HTTP service is
   gone, so a deployment that set one to a non-default value has to move its clients. And `ocdav`,
   previously mounted at the root and therefore answering every unmatched URL, now declares the
   entry points it actually serves, so anything else gets a 404.

   Services no longer expose a prefix at all: the routes say where they serve. The two whose URL a
   peer builds from the registry, the data provider and the data gateway, advertise the path they
   are served under themselves.

   The WebDAV service used to route itself, walking the request path segment by segment through
   nested switches spread over several files, accumulating the base URI as it went and rewriting
   the URL in place. It now declares its entry points, each with the base URI it reports fixed at
   declaration and a handler declared per method, so the router picks the operation and refuses a
   method nobody serves. What is left of the walk is one segment read per subtree, for the user,
   space, share or token its URL carries.

   https://github.com/cs3org/reva/pull/5842

 * Enhancement #5951: Add help-<target> to the Makefile

   `make help-<target>` shows the details of a target: its description, its variables and its
   prerequisites. The details come from the comment block above the target.

   https://github.com/cs3org/reva/pull/5951

 * Enhancement #5950: Add a help target to the Makefile

   `make help` lists the documented targets by section, colored on a terminal. A target is listed
   when its line has a `## description`.

   https://github.com/cs3org/reva/pull/5950

 * Enhancement #5938: Report the progress of the expired shares job

   The job reports the expired shares handled so far, with the removed and failed counts.

   https://github.com/cs3org/reva/pull/5938

 * Enhancement #5937: Report the progress of the orphan reconciliation

   The orphan check reports the shares and links checked so far, with the orphaned, skipped and
   failed counts.

   https://github.com/cs3org/reva/pull/5937

 * Enhancement #5936: Report the progress of the shallow reconciliation

   The shallow check reports its phase, the shares or spaces done so far and its running totals.

   https://github.com/cs3org/reva/pull/5936

 * Enhancement #5947: Rewrite the reva CLI on cobra

   The console mode is removed and commands are noun-verb groups with `--long` flags and shell
   completion. Settings come from flags, env or `reva config set`, and errors give correct exit
   codes.

   https://github.com/cs3org/reva/pull/5947

 * Enhancement #5948: Color the reva CLI help

   The help and the errors of the reva CLI are colored on a terminal. `NO_COLOR` turns the colors
   off, also in the admin tables and logs.

   https://github.com/cs3org/reva/pull/5948

 * Enhancement #5934: Let periodic jobs report progress

   A periodic job can set `RunWithProgress` to report how far it has got, like an on-demand job.

   https://github.com/cs3org/reva/pull/5934

 * Enhancement #5940: Report the progress of the job retention

   The `rjobs.retention` job reports how many runs it pruned so far.

   https://github.com/cs3org/reva/pull/5940

 * Enhancement #5939: Report the progress of the takeout cleanup

   The cleanup job reports the takeout folders checked so far and how many it deleted.

   https://github.com/cs3org/reva/pull/5939

 * Enhancement #5935: Report the progress of takeouts

   The takeout job reports the bytes archived so far, so `reva admin jobs` shows how far a takeout
   has got.

   https://github.com/cs3org/reva/pull/5935


