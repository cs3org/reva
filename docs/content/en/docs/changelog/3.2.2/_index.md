
---
title: "v3.2.2"
linkTitle: "v3.2.2"
weight: 999678
description: >
  Changelog for Reva v3.2.2 (2025-10-15)
---

Changelog for reva 3.2.2 (2025-10-15)
=======================================

The following sections list the changes in reva 3.2.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5362: Use url.JoinPath instead of fmt.Sprintf and path.Join

Details
-------

 * Bugfix #5362: Use url.JoinPath instead of fmt.Sprintf and path.Join

   In some places, we used path.Join instead of url.JoinPath. This leads to missing slashes in the
   http prefix, and improper parsing of the URLs. Instead, we now rely on url.JoinPath

   https://github.com/cs3org/reva/pull/5362


