
---
title: "v2.5.1"
linkTitle: "v2.5.1"
weight: 40
description: >
  Changelog for Reva v2.5.1 (2022-06-08)
---

Changelog for reva 2.5.1 (2022-06-08)
=======================================

The following sections list the changes in reva 2.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2931: Allow listing share jail space
 * Fix #3704: Fix propfinds with depth 0

Details
-------

 * Bugfix #2931: Allow listing share jail space

   Clients can now list the share jail content via `PROPFIND /dav/spaces/{sharejailid}`

   https://github.com/cs3org/reva/pull/2931

 * Bugfix #3704: Fix propfinds with depth 0

   Fixed the response for propfinds with depth 0. The response now doesn't contain the shares jail
   anymore.

   https://github.com/owncloud/ocis/issues/3704
   https://github.com/cs3org/reva/pull/2918


