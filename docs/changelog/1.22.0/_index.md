
---
title: "v1.22.0"
linkTitle: "v1.22.0"
weight: 40
description: >
  Changelog for Reva v1.22.0 (2022-12-31)
---

Changelog for reva 1.22.0 (2022-12-31)
=======================================

The following sections list the changes in reva 1.22.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3528: Fix expired authenticated public link error code
 * Fix #3121: Add missing domain normalization to mentix provider authorizer
 * Enh #3565: Migrate the litmus tests from Drone to GitHub Actions

Details
-------

 * Bugfix #3528: Fix expired authenticated public link error code

   On an expired authenticated public link, the error returned was 401 unauthorized, behaving
   differently from a not-authenticated one, that returns 404 not found. This has been fixed,
   returning 404 not found.

   https://github.com/cs3org/reva/pull/3528

 * Bugfix #3121: Add missing domain normalization to mentix provider authorizer

   The Mentix OCM Provider authorizer lacked provider domain normalization. This led to
   incorrect provider domain matching when authorizing OCM providers.

   https://github.com/cs3org/reva/pull/3121

 * Enhancement #3565: Migrate the litmus tests from Drone to GitHub Actions

   We've migrated the litmusOcisOldWebdav and the litmusOcisNewWebdav tests from Drone to
   GitHub Actions.

   https://github.com/cs3org/reva/pull/3565


