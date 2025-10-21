
---
title: "v3.3.1"
linkTitle: "v3.3.1"
weight: 40
description: >
  Changelog for Reva v3.3.1 (2025-10-21)
---

Changelog for reva 3.3.1 (2025-10-21)
=======================================

The following sections list the changes in reva 3.3.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5376: Deny access shares should not be returned in SharedWithMe call
 * Fix #5375: Fix parsing of OCM Address in case of more than one "@" present

Details
-------

 * Bugfix #5376: Deny access shares should not be returned in SharedWithMe call

   - SharedWithMe no longer returns deny access shares

   https://github.com/cs3org/reva/pull/5376

 * Bugfix #5375: Fix parsing of OCM Address in case of more than one "@" present

   I've fixed the behavior for parsing a long-standing annoyance for users who had OCM Address
   like "mahdi-baghbani@it-department@azadehafzar.io".

   https://github.com/cs3org/reva/pull/5375


