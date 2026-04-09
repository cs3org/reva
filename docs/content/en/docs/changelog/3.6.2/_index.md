
---
title: "v3.6.2"
linkTitle: "v3.6.2"
weight: 999638
description: >
  Changelog for Reva v3.6.2 (2026-03-29)
---

Changelog for reva 3.6.2 (2026-03-29)
=======================================

The following sections list the changes in reva 3.6.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5551: Hide orphaned shares
 * Enh #5549: Add administrative function to list all projects
 * Enh #5546: Ignore files not found in archiver
 * Enh #5538: Cleanup config
 * Enh #5557: Add quota cache

Details
-------

 * Bugfix #5551: Hide orphaned shares

   https://github.com/cs3org/reva/pull/5551

 * Enhancement #5549: Add administrative function to list all projects

   Added ListAllProjects function to the SQL projects manager. The function supports optional
   filtering by status and owner.

   https://github.com/cs3org/reva/pull/5549

 * Enhancement #5546: Ignore files not found in archiver

   https://github.com/cs3org/reva/pull/5546

 * Enhancement #5538: Cleanup config

   https://github.com/cs3org/reva/pull/5538

 * Enhancement #5557: Add quota cache

   Since EOS at the moment takes a long time to respond to `GetQuota` requests, we have implemented
   a cache for the quota results. The cache is warmed up on init, where the cache is populated from a
   single `GetQuota` call that obtains all user quotas.

   https://github.com/cs3org/reva/pull/5557


