
---
title: "v2.16.2"
linkTitle: "v2.16.2"
weight: 40
description: >
  Changelog for Reva v2.16.2 (2023-10-17)
---

Changelog for reva 2.16.2 (2023-10-17)
=======================================

The following sections list the changes in reva 2.16.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4251: GetUserByClaim not working with MSAD for claim "userid"

Details
-------

*   Bugfix #4251: GetUserByClaim not working with MSAD for claim "userid"

   We fixed GetUserByClaim to correctly deal with binary encoded userid as e.g. used for Active
   Directory.

   https://github.com/owncloud/ocis/issues/7469
   https://github.com/cs3org/reva/pull/4251
   https://github.com/cs3org/reva/pull/4249

