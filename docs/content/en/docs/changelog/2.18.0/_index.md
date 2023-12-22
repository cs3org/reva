
---
title: "v2.18.0"
linkTitle: "v2.18.0"
weight: 40
description: >
  Changelog for Reva v2.18.0 (2023-12-22)
---

Changelog for reva 2.18.0 (2023-12-22)
=======================================

The following sections list the changes in reva 2.18.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4424: Fixed panic in receivedsharecache pkg
*   Fix #4425: Fix overwriting files with empty files
*   Fix #4432: Fix /dav/meta endpoint for shares
*   Fix #4422: Fix disconnected traces
*   Fix #4429: Internal link creation
*   Fix #4407: Make ocdav return correct oc:spaceid
*   Fix #4410: Improve OCM support
*   Fix #4402: Refactor upload session
*   Enh #4421: Check permissions before adding, deleting or updating shares
*   Enh #4403: Add validation to update public share
*   Enh #4409: Disable the password policy
*   Enh #4412: Allow authentication for nats connections
*   Enh #4411: Add option to configure streams non durable
*   Enh #4406: Rework cache configuration
*   Enh #4414: Track more upload session metrics

Details
-------

*   Bugfix #4424: Fixed panic in receivedsharecache pkg

   The receivedsharecache pkg would sometime run into concurrent map writes. This is fixed by
   using maptimesyncedcache pkg instead of a plain map.

   https://github.com/cs3org/reva/pull/4424

*   Bugfix #4425: Fix overwriting files with empty files

   We fixed a bug where files could not be overwritten with empty files using the desktop client.

   https://github.com/cs3org/reva/pull/4425

*   Bugfix #4432: Fix /dav/meta endpoint for shares

   We fixed a bug in the /dav/meta endpoint leading to internal server errors when used with
   shares.

   https://github.com/cs3org/reva/pull/4432

*   Bugfix #4422: Fix disconnected traces

   We fixed a problem where the appctx logger was using a new traceid instead of picking up the one
   from the trace parent.

   https://github.com/cs3org/reva/pull/4422

*   Bugfix #4429: Internal link creation

   We fix the permission checks for creating and updating public share so that it is possible again
   to create internal links for received shares.

   https://github.com/owncloud/ocis/issues/8039
   https://github.com/cs3org/reva/pull/4429

*   Bugfix #4407: Make ocdav return correct oc:spaceid

   Propfinds now return `oc:spaceid` in the form of `{providerid}${spaceid}`

   https://github.com/cs3org/reva/pull/4407

*   Bugfix #4410: Improve OCM support

   We fixed several bugs with OCM support.

   https://github.com/cs3org/reva/pull/4410
   https://github.com/cs3org/reva/pull/4333

*   Bugfix #4402: Refactor upload session

   We refactored the upload session code to make it reusable, kill a lot of code and save some stat
   requests

   https://github.com/cs3org/reva/pull/4402

*   Enhancement #4421: Check permissions before adding, deleting or updating shares

   The user share provider now checks if the user has sufficient permissions to add, delete or
   update a share.

   https://github.com/cs3org/reva/pull/4421
   https://github.com/cs3org/reva/pull/4405

*   Enhancement #4403: Add validation to update public share

   We added validation to update public share provider to move the logic from the handlers to the
   implementing server.

   https://github.com/cs3org/reva/pull/4403

*   Enhancement #4409: Disable the password policy

   We add the environment variable that allow to disable the password policy.

   https://github.com/cs3org/reva/pull/4409

*   Enhancement #4412: Allow authentication for nats connections

   Allows configuring username/password for nats connections

   https://github.com/cs3org/reva/pull/4412

*   Enhancement #4411: Add option to configure streams non durable

   Adds an option to disable persistence of event streams

   https://github.com/cs3org/reva/pull/4411

*   Enhancement #4406: Rework cache configuration

   Reworks configuration of the cache package allowing easier configuration. Also adds a new
   config value allow to not persist cache entries (nats only)

   https://github.com/cs3org/reva/pull/4406

*   Enhancement #4414: Track more upload session metrics

   We added a gauge for the number of uploads currently in postprocessing as well as counters for
   different postprocessing outcomes.

   https://github.com/cs3org/reva/pull/4414

