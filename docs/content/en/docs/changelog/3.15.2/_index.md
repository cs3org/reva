
---
title: "v3.15.2"
linkTitle: "v3.15.2"
weight: 999548
description: >
  Changelog for Reva v3.15.2 (2026-11-12)
---

Changelog for reva 3.15.2 (2026-11-12)
=======================================

The following sections list the changes in reva 3.15.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #6005: Try upload on file itself first

Details
-------

 * Bugfix #6005: Try upload on file itself first

   We should not try to access the parent folder if the file already exists; this results in
   PermissionDenied for single file shares

   https://github.com/cs3org/reva/pull/6005


