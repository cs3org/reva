
---
title: "v2.13.2"
linkTitle: "v2.13.2"
weight: 40
description: >
  Changelog for Reva v2.13.2 (2023-05-08)
---

Changelog for reva 2.13.2 (2023-05-08)
=======================================

The following sections list the changes in reva 2.13.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3845: Fix propagation
*   Fix #3856: Fix response code
*   Fix #3857: Fix trashbin purge

Details
-------

*   Bugfix #3845: Fix propagation

   Fix propagation in concurrency scenarios

   https://github.com/cs3org/reva/pull/3845

*   Bugfix #3856: Fix response code

   The DeleteStorageSpace method response code has been changed

   https://github.com/cs3org/reva/pull/3856

*   Bugfix #3857: Fix trashbin purge

   We have fixed a nil-pointer-exception, when purging files from the trashbin that do not have a
   parent (any more)

   https://github.com/owncloud/ocis/issues/6245
   https://github.com/cs3org/reva/pull/3857

