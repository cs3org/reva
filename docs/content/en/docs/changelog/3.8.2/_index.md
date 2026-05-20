
---
title: "v3.8.2"
linkTitle: "v3.8.2"
weight: 999618
description: >
  Changelog for Reva v3.8.2 (2026-05-20)
---

Changelog for reva 3.8.2 (2026-05-20)
=======================================

The following sections list the changes in reva 3.8.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5619: Fix EOS bug in version folder creation

Details
-------

 * Bugfix #5619: Fix EOS bug in version folder creation

   Due to a bug in the EOS drivers, version folders were created under the owner of the first
   resource in a directory, instead of the owner of the corresponding file.

   https://github.com/cs3org/reva/pull/5619


