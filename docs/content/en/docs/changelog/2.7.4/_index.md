
---
title: "v2.7.4"
linkTitle: "v2.7.4"
weight: 40
description: >
  Changelog for Reva v2.7.4 (2022-08-10)
---

Changelog for reva 2.7.4 (2022-08-10)
=======================================

The following sections list the changes in reva 2.7.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3141: Check ListGrants permission when listing shares

Details
-------

*   Bugfix #3141: Check ListGrants permission when listing shares

   We now check the ListGrants permission when listing outgoing shares. If this permission is
   set, users can list all shares in all spaces.

   https://github.com/cs3org/reva/pull/3141


