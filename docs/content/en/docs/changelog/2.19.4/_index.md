
---
title: "v2.19.4"
linkTitle: "v2.19.4"
weight: 40
description: >
  Changelog for Reva v2.19.4 (2024-04-05)
---

Changelog for reva 2.19.4 (2024-04-05)
=======================================

The following sections list the changes in reva 2.19.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4612: Use gateway selector in jsoncs3

Details
-------

*   Bugfix #4612: Use gateway selector in jsoncs3

   The jsoncs3 user share manager now uses the gateway selector to get a fresh client before making
   requests and uses the configured logger from the context.

   https://github.com/cs3org/reva/pull/4612

