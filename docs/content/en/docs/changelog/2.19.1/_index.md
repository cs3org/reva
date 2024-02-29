
---
title: "v2.19.1"
linkTitle: "v2.19.1"
weight: 40
description: >
  Changelog for Reva v2.19.1 (2024-02-29)
---

Changelog for reva 2.19.1 (2024-02-29)
=======================================

The following sections list the changes in reva 2.19.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4534: Fix remove/update share permissions
*   Fix #4539: Fix a typo

Details
-------

*   Bugfix #4534: Fix remove/update share permissions

   This is a workaround that should prevent removing or changing the share permissions when the
   file is locked. These limitations have to be removed after the wopi server will be able to unlock
   the file properly. These limitations are not spread on the files inside the shared folder.

   https://github.com/owncloud/ocis/issues/8273
   https://github.com/cs3org/reva/pull/4534

*   Bugfix #4539: Fix a typo

   We fixed a typo

   https://github.com/cs3org/reva/pull/4539

