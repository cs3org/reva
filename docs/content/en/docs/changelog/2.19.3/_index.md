
---
title: "v2.19.3"
linkTitle: "v2.19.3"
weight: 40
description: >
  Changelog for Reva v2.19.3 (2024-04-04)
---

Changelog for reva 2.19.3 (2024-04-04)
=======================================

The following sections list the changes in reva 2.19.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4607: Mask user email in output

Details
-------

*   Bugfix #4607: Mask user email in output

   We have fixed a bug where the user email was not masked in the output and the user emails could be
   enumerated through the sharee search.

   https://github.com/owncloud/ocis/issues/8726
   https://github.com/cs3org/reva/pull/4607
   https://github.com/cs3org/reva/pull/4603

