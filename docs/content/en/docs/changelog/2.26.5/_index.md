
---
title: "v2.26.5"
linkTitle: "v2.26.5"
weight: 40
description: >
  Changelog for Reva v2.26.5 (2024-11-12)
---

Changelog for reva 2.26.5 (2024-11-12)
=======================================

The following sections list the changes in reva 2.26.5 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4926: Make etag always match content on downloads
*   Fix #4920: Return correct status codes for simple uploads
*   Fix #4924: Fix sync propagation
*   Fix #4916: Improve posixfs stability and performance

Details
-------

*   Bugfix #4926: Make etag always match content on downloads

   We added an openReaderfunc to the Download interface to give drivers a way to guarantee that the
   reader matches the etag returned in a previous GetMD call.

   https://github.com/cs3org/reva/pull/4926
   https://github.com/cs3org/reva/pull/4923

*   Bugfix #4920: Return correct status codes for simple uploads

   Decomposedfs now returns the correct precondition failed status code when the etag does not
   match. This allows the jsoncs3 share managers optimistic locking to handle concurrent writes
   correctly

   https://github.com/cs3org/reva/pull/4920

*   Bugfix #4924: Fix sync propagation

   Fixes the defers in the sync propagation.

   https://github.com/cs3org/reva/pull/4924

*   Bugfix #4916: Improve posixfs stability and performance

   The posixfs storage driver saw a number of bugfixes and optimizations.

   https://github.com/cs3org/reva/pull/4916

