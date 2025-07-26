
---
title: "v1.28.0"
linkTitle: "v1.28.0"
weight: 40
description: >
  Changelog for Reva v1.28.0 (2024-02-27)
---

Changelog for reva 1.28.0 (2024-02-27)
=======================================

The following sections list the changes in reva 1.28.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4369: Carefully use root credentials to perform system level ops
 * Fix #4306: Correctly treat EOS urls containing # chars
 * Fix #4510: Propagates traceID to EOS
 * Fix #4321: Reworked List() to support version folder tricks
 * Fix #4400: Fix group-based capabilities
 * Fix #4319: Fixed registration of custom extensions in the mime registry
 * Fix #4287: Fixes registration and naming of services
 * Fix #4310: Restore changes to ceph driver
 * Fix #4294: Sciencemesh fixes
 * Fix #4307: Dynamic storage registry storage_id aliases
 * Fix #4497: Removed stat to all storage providers on Depth:0 PROPFIND to "/"
 * Enh #4280: Implementation of Locks for the CephFS driver
 * Enh #4282: Support multiple templates in config entries
 * Enh #4304: Disable open in app for given paths
 * Enh #4455: Limit max number of entries returned by ListRecycle in eos
 * Enh #4309: Get the logger in the grpcMDResponseToFileInfo func, log the stat
 * Enh #4311: Init time logger for eosgrpc storage driver
 * Enh #4301: Added listversions command
 * Enh #4493: Removed notification capability
 * Enh #4288: Print plugins' version
 * Enh #4508: Add pprof http service
 * Enh #4376: Removed cback from upstream codebase
 * Enh #4391: CERNBox setup for ScienceMesh tests
 * Enh #4246: Revamp ScienceMesh integration tests
 * Enh #4240: Reworked protocol with ScienceMesh NC/OC apps
 * Enh #4370: Storage registry: fail at init if config is missing any providers

Details
-------

 * Bugfix #4369: Carefully use root credentials to perform system level ops

   This PR ensures that system level ops like setlock, setattr, stat... work when invoked from a
   gateway This is relevant for eosgrpc, as eosbinary exploited the permissivity of the eos
   cmdline

   https://github.com/cs3org/reva/pull/4369

 * Bugfix #4306: Correctly treat EOS urls containing # chars

   https://github.com/cs3org/reva/pull/4306

 * Bugfix #4510: Propagates traceID to EOS

   This PR fixes the cases where the EOS trace ID was always a bunch of zeroes.

   https://github.com/cs3org/reva/pull/4510

 * Bugfix #4321: Reworked List() to support version folder tricks

   https://github.com/cs3org/reva/pull/4321

 * Bugfix #4400: Fix group-based capabilities

   The group-based capabilities require an authenticated endpoint, as we must query the
   logged-in user's groups to get those. This PR moves them to the `getSelf` endpoint in the user
   handler.

   https://github.com/cs3org/reva/pull/4400

 * Bugfix #4319: Fixed registration of custom extensions in the mime registry

   This PR ensures custom extensions/mime-types are registered by trimming any eventual
   leading '.' from the extension.

   https://github.com/cs3org/reva/pull/4319

 * Bugfix #4287: Fixes registration and naming of services

   https://github.com/cs3org/reva/pull/4287

 * Bugfix #4310: Restore changes to ceph driver

   PR [4166](https://github.com/cs3org/reva/pull/4166) accidentally reverted the ceph
   driver changes. This PR recovers them.

   https://github.com/cs3org/reva/pull/4310

 * Bugfix #4294: Sciencemesh fixes

   Fixes different issues introduced with the recent changes, in ocm/sciencemesh, in
   particular the `GetAccepetdUser` and `/sciencemesh/find-accepted-users` endpoints.

   https://github.com/cs3org/reva/pull/4294

 * Bugfix #4307: Dynamic storage registry storage_id aliases

   Fixes the bug where the dynamic storage registry would not be able to resolve storage ids like
   `eoshome-a`, as those are aliased and need to be resolved into the proper storage-id
   (`eoshome-i01`).

   https://github.com/cs3org/reva/pull/4307

 * Bugfix #4497: Removed stat to all storage providers on Depth:0 PROPFIND to "/"

   This PR removes an unnecessary and potentially problematic call, which would fail if any of the
   configured storage providers has an issue.

   https://github.com/cs3org/reva/pull/4497

 * Enhancement #4280: Implementation of Locks for the CephFS driver

   This PR brings CS3APIs Locks for CephFS

   https://github.com/cs3org/reva/pull/4280

 * Enhancement #4282: Support multiple templates in config entries

   This PR introduces support for config entries with multiple templates, such as `parameter =
   "{{ vars.v1 }} foo {{ vars.v2 }}"`. Previously, only one `{{ template }}` was allowed in a given
   configuration entry.

   https://github.com/cs3org/reva/pull/4282

 * Enhancement #4304: Disable open in app for given paths

   https://github.com/cs3org/reva/pull/4304

 * Enhancement #4455: Limit max number of entries returned by ListRecycle in eos

   The idea is to query first how many entries we'd have from eos recycle ls and bail out if "too
   many".

   https://github.com/cs3org/reva/pull/4455

 * Enhancement #4309: Get the logger in the grpcMDResponseToFileInfo func, log the stat

   https://github.com/cs3org/reva/pull/4309

 * Enhancement #4311: Init time logger for eosgrpc storage driver

   Before the `eosgrpc` driver was using a custom logger. Now that the reva logger is available at
   init time, the driver will use this.

   https://github.com/cs3org/reva/pull/4311

 * Enhancement #4301: Added listversions command

   https://github.com/cs3org/reva/pull/4301

 * Enhancement #4493: Removed notification capability

   This is not needed any longer, the code was simplified to enable notifications if they are
   configured

   https://github.com/cs3org/reva/pull/4493

 * Enhancement #4288: Print plugins' version

   https://github.com/cs3org/reva/pull/4288

 * Enhancement #4508: Add pprof http service

   This service is useful to trigger diagnostics on running processes

   https://github.com/cs3org/reva/pull/4508

 * Enhancement #4376: Removed cback from upstream codebase

   The code has been moved to as a CERNBox plugin.

   https://github.com/cs3org/reva/pull/4376

 * Enhancement #4391: CERNBox setup for ScienceMesh tests

   This PR includes a bundled CERNBox-like web UI and backend to test the ScienceMesh workflows
   with OC10 and NC

   https://github.com/cs3org/reva/pull/4391

 * Enhancement #4246: Revamp ScienceMesh integration tests

   This extends the ScienceMesh tests by running a wopiserver next to each EFSS/IOP, and by
   including a CERNBox-like minimal configuration. The latter is based on local storage and
   in-memory shares (no db dependency).

   https://github.com/cs3org/reva/pull/4246

 * Enhancement #4240: Reworked protocol with ScienceMesh NC/OC apps

   This ensures full OCM 1.1 coverage

   https://github.com/cs3org/reva/pull/4240

 * Enhancement #4370: Storage registry: fail at init if config is missing any providers

   This change makes the dynamic storage registry fail at startup if there are missing rules in the
   config file. That is, any `mount_id` in the routing table must have a corresponding
   `storage_id`/`address` pair in the config, otherwise the registry will fail to start.

   https://github.com/cs3org/reva/pull/4370


