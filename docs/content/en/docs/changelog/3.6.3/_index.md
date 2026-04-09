
---
title: "v3.6.3"
linkTitle: "v3.6.3"
weight: 999637
description: >
  Changelog for Reva v3.6.3 (2026-04-07)
---

Changelog for reva 3.6.3 (2026-04-07)
=======================================

The following sections list the changes in reva 3.6.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5570: Remove broken EOS token cache

Details
-------

 * Bugfix #5570: Remove broken EOS token cache

   There was a bug in the EOS token cache that caused infinite loops. The token cache has been
   completely removed, since we will be moving away from EOS tokens soon anyway.

   https://github.com/cs3org/reva/pull/5570


