
---
title: "v3.4.2"
linkTitle: "v3.4.2"
weight: 999658
description: >
  Changelog for Reva v3.4.2 (2025-12-17)
---

Changelog for reva 3.4.2 (2025-12-17)
=======================================

The following sections list the changes in reva 3.4.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5444: Allow lightweight accounts to use external apps
 * Enh #5442: Refactor emailhandler

Details
-------

 * Bugfix #5444: Allow lightweight accounts to use external apps

   For that, we need to explicitly allow all relevant storage provider requests when checking the
   lw scope.

   https://github.com/cs3org/reva/pull/5444

 * Enhancement #5442: Refactor emailhandler

   The email handler now supports reading CID files and embedding them.

   https://github.com/cs3org/reva/pull/5442


