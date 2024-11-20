
---
title: "v2.26.7"
linkTitle: "v2.26.7"
weight: 40
description: >
  Changelog for Reva v2.26.7 (2024-11-20)
---

Changelog for reva 2.26.7 (2024-11-20)
=======================================

The following sections list the changes in reva 2.26.7 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4964: Fix a wrong error code when approvider creates a new file

Details
-------

*   Bugfix #4964: Fix a wrong error code when approvider creates a new file

   We fixed a problem where the approvider would return a 500 error instead of 403 when trying to
   create a new file in a read-only share.

   https://github.com/cs3org/reva/pull/4964

