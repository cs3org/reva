
---
title: "v2.19.5"
linkTitle: "v2.19.5"
weight: 40
description: >
  Changelog for Reva v2.19.5 (2024-04-17)
---

Changelog for reva 2.19.5 (2024-04-17)
=======================================

The following sections list the changes in reva 2.19.5 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4626: Fix public share update
*   Fix #4634: Fix access to files withing a public link targeting a space root

Details
-------

*   Bugfix #4626: Fix public share update

   We fixed the permission check for updating public shares. When updating the permissions of a
   public share while not providing a password, the check must be against the new permissions to
   take into account that users can opt out only for view permissions.

   https://github.com/cs3org/reva/pull/4626
   https://github.com/cs3org/reva/pull/4622

*   Bugfix #4634: Fix access to files withing a public link targeting a space root

   We fixed an issue that prevented users from opening documents within a public share that
   targets a space root.

   https://github.com/owncloud/ocis/issues/8691
   https://github.com/cs3org/reva/pull/4634/
   https://github.com/cs3org/reva/pull/4632/

