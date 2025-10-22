
---
title: "v1.27.0"
linkTitle: "v1.27.0"
weight: 998730
description: >
  Changelog for Reva v1.27.0 (2023-10-19)
---

Changelog for reva 1.27.0 (2023-10-19)
=======================================

The following sections list the changes in reva 1.27.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4196: Access public links to projects as owner
 * Enh #4266: Improve authentication routing logic
 * Enh #4212: CERNBox cleanup
 * Enh #4199: Dynamic storage provider
 * Enh #4264: Implement eos-compliant app locks
 * Enh #4200: Multiple fixes for Ceph driver
 * Enh #4185: Refurbish the grpc and https plugins for eos
 * Enh #4166: Add better observability with metrics and traces
 * Enh #4195: Support incoming OCM 1.0 shares
 * Enh #4189: Support full URL endpoints in ocm-provider
 * Enh #4186: Fixes in the reference configuration for ScienceMesh
 * Enh #4191: Add metrics service to ScienceMesh example config

Details
-------

 * Bugfix #4196: Access public links to projects as owner

   https://github.com/cs3org/reva/pull/4196

 * Enhancement #4266: Improve authentication routing logic

   Provides a safer approach to route requests, both in HTTP and gRPC land when authentication is
   needed.

   https://github.com/cs3org/reva/pull/4266

 * Enhancement #4212: CERNBox cleanup

   Remove from the codebase all the cernbox specific code

   https://github.com/cs3org/reva/pull/4212

 * Enhancement #4199: Dynamic storage provider

   Add a new storage provider that can globally route to other providers. This provider uses a
   routing table in the database containing `path` - `mountid` pairs, and a mapping `mountid` -
   `address` in the config. It also support rewriting paths for resolution (to enable more
   complex cases).

   https://github.com/cs3org/reva/pull/4199

 * Enhancement #4264: Implement eos-compliant app locks

   The eosfs package now uses the app locks provided by eos

   https://github.com/cs3org/reva/pull/4264

 * Enhancement #4200: Multiple fixes for Ceph driver

   * Avoid usage/creation of user homes when they are disabled in the config * Simplify the regular
   uploads (not chunked) * Avoid creation of shadow folders at the root if they are already there *
   Clean up the chunked upload * Fix panic on shutdown

   https://github.com/cs3org/reva/pull/4200

 * Enhancement #4185: Refurbish the grpc and https plugins for eos

   This enhancement refurbishes the grpc and https plugins for eos

   https://github.com/cs3org/reva/pull/4185

 * Enhancement #4166: Add better observability with metrics and traces

   Adds prometheus collectors that can be registered dynamically and also refactors the http and
   grpc clients and servers to propage trace info.

   https://github.com/cs3org/reva/pull/4166

 * Enhancement #4195: Support incoming OCM 1.0 shares

   OCM 1.0 payloads are now supported as incoming shares, and converted to the OCM 1.1 format for
   persistency and further processing. Outgoing shares are still only OCM 1.1.

   https://github.com/cs3org/reva/pull/4195

 * Enhancement #4189: Support full URL endpoints in ocm-provider

   This patch enables a reva server to properly show any configured endpoint route in all relevant
   properties exposed by /ocm-provider. This allows reverse proxy configurations of the form
   https://server/route to be supported for the OCM discovery mechanism.

   https://github.com/cs3org/reva/pull/4189

 * Enhancement #4186: Fixes in the reference configuration for ScienceMesh

   Following the successful onboarding of CESNET, this PR brings some improvements and fixes to
   the reference configuration, as well as some adaptation to the itegration tests.

   https://github.com/cs3org/reva/pull/4186
   https://github.com/cs3org/reva/pull/4184
   https://github.com/cs3org/reva/pull/4183

 * Enhancement #4191: Add metrics service to ScienceMesh example config

   Adds the metrics http service configuration to the example config file of a ScienceMesh site.
   Having this service configured is a prerequisite for successfull Prometheus-based
   ScienceMesh sites metrics scraping.

   https://github.com/cs3org/reva/pull/4191


