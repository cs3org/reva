
---
title: "v3.10.2"
linkTitle: "v3.10.2"
weight: 999598
description: >
  Changelog for Reva v3.10.2 (2026-06-29)
---

Changelog for reva 3.10.2 (2026-06-29)
=======================================

The following sections list the changes in reva 3.10.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5650: Fix for cephmount lock & xattr bugs
 * Fix #5649: Ceph: fix path resolution
 * Fix #5648: Eosfs: fixed lock retrieval
 * Fix #5673: Fix a nilptr in the HTTP server
 * Fix #5662: Check for nil pointers
 * Fix #5676: Fixed parsing of OCM discovery payload
 * Fix #5681: Correctly identify OCM embedded payloads
 * Fix #5653: Honor pidfile passed with the -p option
 * Fix #5639: Bound the Prometheus HTTP handler label cardinality
 * Enh #5651: Add a background jobs framework
 * Enh #5663: Add immutable stubs & upgrade cs3apis
 * Enh #5652: Add support for signed URLs of the archiver
 * Enh #5645: Implement hiding for ocm shares
 * Enh #5611: OCM: reworked discovery logic and payload
 * Enh #5654: Enable external accounts to list received OCM shares
 * Enh #5664: Implement the new OCM webapp protocol
 * Enh #5674: Honor outbound proxy settings in the OCM client
 * Enh #5643: Utilize the new transferring state in the cs3api

Details
-------

 * Bugfix #5650: Fix for cephmount lock & xattr bugs

   * ReadArbitraryMetadata - Returns stored user.* xattrs in GetMD instead of nothing. * Paths
   Unlock/SetLock open the full chroot path instead of the incorrect relative path. * Quality of
   life improvements for error handling & debug logs.

   https://github.com/cs3org/reva/pull/5650

 * Bugfix #5649: Ceph: fix path resolution

   When performing a id-to-path lookup the path may not always be empty, as in the case for wopi,
   this case was not handled.

   https://github.com/cs3org/reva/pull/5649

 * Bugfix #5648: Eosfs: fixed lock retrieval

   GetLock on a file with an empty lock now returns no lock as opposed to failing with a malformed
   lock exception

   https://github.com/cs3org/reva/pull/5648

 * Bugfix #5673: Fix a nilptr in the HTTP server

   https://github.com/cs3org/reva/pull/5673

 * Bugfix #5662: Check for nil pointers

   https://github.com/cs3org/reva/pull/5662

 * Bugfix #5676: Fixed parsing of OCM discovery payload

   The principle is that we should not fail the JSON parsing on unsupported capabilities, but only
   on out of spec payloads. Therefore, `ResourceType` must be a generic string, to be validated
   afterwards.

   https://github.com/cs3org/reva/pull/5676

 * Bugfix #5681: Correctly identify OCM embedded payloads

   OCM "ro-crate" shares are to be mapped to (CS3) EMBEDDED resource types, where "embedded" is a
   generic term to signal that any JSON-represented payload can be mapped in this way.

   https://github.com/cs3org/reva/pull/5681

 * Bugfix #5653: Honor pidfile passed with the -p option

   The pidfile was always generated at a random path in the OS temp dir on startup, ignoring the
   location passed with -p. As a result -s reload (and the other -s signals) could not find the
   running master to signal it. The -p path is now honored when starting revad.

   https://github.com/cs3org/reva/pull/5653

 * Bugfix #5639: Bound the Prometheus HTTP handler label cardinality

   The HTTP metrics interceptor used the full request URL path as the value of the `handler` label
   on the `http_request_duration_seconds` histogram. Because Reva HTTP endpoints are
   user-bound, this produced unbounded label cardinality and caused excessive metrics
   storage. The label is now derived from the leading static path segment, keeping cardinality
   bounded to the number of route prefixes.

   https://github.com/cs3org/reva/pull/5639%60%60%60

 * Enhancement #5651: Add a background jobs framework

   Introduce a framework to run background work in reva, both periodically (e.g. warming a cache
   or cleaning up expired state) and once on demand on a user request. Jobs are hosted by a new "jobs"
   serverless service backed by NATS JetStream, run status is tracked in a SQL store and can be
   queried by run id, and multiple jobs processes can run together without duplicating work.

   https://github.com/cs3org/reva/pull/5651

 * Enhancement #5663: Add immutable stubs & upgrade cs3apis

   This upgrade is needed so we can move past this version to the cs3apis even though we don't use it
   currently.

   https://github.com/cs3org/reva/pull/5663

 * Enhancement #5652: Add support for signed URLs of the archiver

   https://github.com/cs3org/reva/pull/5652

 * Enhancement #5645: Implement hiding for ocm shares

   This commit introduces the functionailty required to hide ocm shares

   https://github.com/cs3org/reva/pull/5645%60

 * Enhancement #5611: OCM: reworked discovery logic and payload

   This PR adapts the OCM Discovery endpoint to the latest standard and introduces better logic to
   discover remote endpoints' protocols.

   https://github.com/cs3org/reva/pull/5611

 * Enhancement #5654: Enable external accounts to list received OCM shares

   External accounts cannot create shares but can be recipient of shares. This now applies to OCM
   shares as well.

   https://github.com/cs3org/reva/pull/5654

 * Enhancement #5664: Implement the new OCM webapp protocol

   Following the OCM specifications, the webapp protocol and access method were reworked in the
   cs3apis: the view mode was replaced by share permissions (view, read, write, share on the
   wire), and the protocol now carries a shared secret, requirements (including
   must-exchange-token), targets (blank or iframe), and optional display metadata (appName,
   appIconHint, mediaTypes).

   - The OCM /shares endpoint now validates and parses webapp protocol payloads according to the
   new specification; legacy payloads carrying a viewMode are rejected - Outgoing webapp shares
   are serialized with the new wire fields, with the shared secret taken from the share token - The
   SQL share manager persists the new webapp fields - Roles for webapp-only shares are now derived
   from the share permissions instead of the view mode

   https://github.com/cs3org/reva/pull/5664

 * Enhancement #5674: Honor outbound proxy settings in the OCM client

   `ocmd.NewClient` now honors the standard HTTP_PROXY, HTTPS_PROXY, and NO_PROXY
   environment variables for outbound OCM requests, while keeping the existing request timeout
   and insecure TLS behavior. This applies to discovery, outgoing shares, invite-accepted,
   token exchange, and directory-service fetches that go through the shared OCM client.

   https://github.com/cs3org/reva/pull/5674

 * Enhancement #5643: Utilize the new transferring state in the cs3api

   - Puts the ocm share in a transferring state when doing the processing of an embedded share is
   ongoing - A callback is implemented that puts the share in the accepted state when the transfer
   is finished

   https://github.com/cs3org/reva/pull/5643


