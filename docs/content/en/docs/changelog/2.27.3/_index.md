
---
title: "v2.27.3"
linkTitle: "v2.27.3"
weight: 40
description: >
  Changelog for Reva v2.27.3 (2025-01-27)
---

Changelog for reva 2.27.3 (2025-01-27)
=======================================

The following sections list the changes in reva 2.27.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #5042: Fix ocis dependency
*   Enh #5051: Emit SpaceMembershipExpired event

Details
-------

*   Bugfix #5042: Fix ocis dependency

   Fix the ocm gateway connection pool. Fix ocis dependency in the reva go.mod file. Bump the ocis
   version accordingly to the major version.

   https://github.com/owncloud/ocis/issues/10846
   https://github.com/owncloud/ocis/issues/10878
   https://github.com/cs3org/reva/pull/5042

*   Enhancement #5051: Emit SpaceMembershipExpired event

   https://github.com/owncloud/ocis/issues/10919
   https://github.com/cs3org/reva/pull/5051

