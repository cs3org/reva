
---
title: "v1.8.1"
linkTitle: "v1.8.1"
weight: 40
description: >
  Changelog for Reva v1.8.1 (2021-06-23)
---

Changelog for reva 1.8.1 (2021-06-23)
=======================================

The following sections list the changes in reva 1.8.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests
 * Fix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI
 * Chg #1721: Absolute and relative references
 * Enh #1774: Add user ID cache warmup to EOS storage driver
 * Enh #1471: EOEGrpc progress. Logging discipline and error handling
 * Enh #1811: Harden public shares signing
 * Enh #1793: Remove the user id from the trashbin key
 * Enh #1795: Increase trashbin restore API compatibility
 * Enh #1516: Use UidNumber and GidNumber fields in User objects

Details
-------

 * Bugfix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests

   Replace the backquotes that were used for the date component of the URL with the
   POSIX-confirmant command substitution '$()'.

   https://github.com/cs3org/reva/pull/1815

 * Bugfix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI

   Some of the dependencies used by revad need CGO to be enabled in order to work. We also need to
   install the 'mime-types' in alpine to correctly detect them on the storage-providers.

   The CGO_ENABLED=0 flag was added to the docker build flags so that it will produce a static
   build. This allows usage of the 'scratch' image for reduction of the docker image size (e.g. the
   reva cli).

   https://github.com/cs3org/reva/issues/1765
   https://github.com/cs3org/reva/pull/1766
   https://github.com/cs3org/reva/pull/1797

 * Change #1721: Absolute and relative references

   We unified the `Reference_Id` end `Reference_Path` types to a combined `Reference` that
   contains both: - a `resource_id` property that can identify a node using a `storage_id` and an
   `opaque_id` - a `path` property that can be used to represent absolute paths as well as paths
   relative to the id based properties. While this is a breaking change it allows passing both:
   absolute as well as relative references.

   https://github.com/cs3org/reva/pull/1721

 * Enhancement #1774: Add user ID cache warmup to EOS storage driver

   https://github.com/cs3org/reva/pull/1774

 * Enhancement #1471: EOEGrpc progress. Logging discipline and error handling

   https://github.com/cs3org/reva/pull/1471

 * Enhancement #1811: Harden public shares signing

   Makes golangci-lint happy as well

   https://github.com/cs3org/reva/pull/1811

 * Enhancement #1793: Remove the user id from the trashbin key

   We don't want to use the users uuid outside of the backend so I removed the id from the trashbin
   file key.

   https://github.com/cs3org/reva/pull/1793

 * Enhancement #1795: Increase trashbin restore API compatibility

   * The precondition were not checked before doing a trashbin restore in the ownCloud dav API.
   Without the checks the API would behave differently compared to the oC10 API. * The restore
   response was missing HTTP headers like `ETag` * Update the name when restoring the file from
   trashbin to a new target name

   https://github.com/cs3org/reva/pull/1795

 * Enhancement #1516: Use UidNumber and GidNumber fields in User objects

   Update instances where CS3API's `User` objects are created and used to use `GidNumber`, and
   `UidNumber` fields instead of storing them in `Opaque` map.

   https://github.com/cs3org/reva/issues/1516


