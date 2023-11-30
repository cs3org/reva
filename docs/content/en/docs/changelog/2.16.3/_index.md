
---
title: "v2.16.3"
linkTitle: "v2.16.3"
weight: 40
description: >
  Changelog for Reva v2.16.3 (2023-11-30)
---

Changelog for reva 2.16.3 (2023-11-30)
=======================================

The following sections list the changes in reva 2.16.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Enh #4377: Handle trashbin file listings concurrently

Details
-------

*   Enhancement #4377: Handle trashbin file listings concurrently

   We now use a concurrent walker to list files in the trashbin. This improves performance when
   listing files in the trashbin.

   https://github.com/owncloud/ocis/issues/7844
   https://github.com/cs3org/reva/pull/4377
   https://github.com/cs3org/reva/pull/4374

