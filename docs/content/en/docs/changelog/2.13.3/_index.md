
---
title: "v2.13.3"
linkTitle: "v2.13.3"
weight: 40
description: >
  Changelog for Reva v2.13.3 (2023-05-17)
---

Changelog for reva 2.13.3 (2023-05-17)
=======================================

The following sections list the changes in reva 2.13.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3890: Bring back public link sharing of project space roots
*   Fix #3888: We fixed a bug that unnecessarily fetched all members of a group
*   Fix #3886: Decomposedfs no longer deadlocks when cache is disabled
*   Fix #3892: Fix public links
*   Fix #3876: Remove go-micro/store/redis specific workaround
*   Fix #3889: Update space root mtime when changing space metadata
*   Fix #3836: Fix spaceID in the decomposedFS
*   Fix #3867: Restore last version after positive result
*   Fix #3849: Prevent sharing space roots and personal spaces
*   Enh #3865: Remove unneccessary code from gateway
*   Enh #3895: Add missing expiry date to shares

Details
-------

*   Bugfix #3890: Bring back public link sharing of project space roots

   We reenabled sharing of project space roots

   https://github.com/cs3org/reva/pull/3890

*   Bugfix #3888: We fixed a bug that unnecessarily fetched all members of a group

   Adding or removing groups to spaces is now done without retrieving all group members

   https://github.com/cs3org/reva/pull/3888

*   Bugfix #3886: Decomposedfs no longer deadlocks when cache is disabled

   We now pass a Reader for the locked file to lower level functions so metadata can be read without
   aquiring a new file lock.

   https://github.com/cs3org/reva/pull/3886

*   Bugfix #3892: Fix public links

   Public links would not work when not send on the root level. Reason was wrong path matching. Also
   fixes a critical bug that was unfound before

   https://github.com/cs3org/reva/pull/3892

*   Bugfix #3876: Remove go-micro/store/redis specific workaround

   We submitted an upstream fix for an issue in the go-micro/store redis plugin. Which allowed us
   to remove a redis specific workaround from the reva storage cache implementation.

   https://github.com/cs3org/reva/pull/3876

*   Bugfix #3889: Update space root mtime when changing space metadata

   We fixed a problem where space mtimes were not updated when their metadata changed, resulting
   in changes not being picked up by other services like search.

   https://github.com/owncloud/ocis/issues/6289
   https://github.com/cs3org/reva/pull/3889

*   Bugfix #3836: Fix spaceID in the decomposedFS

   We returned the wrong spaceID within ``storageSpaceFromNode``. This was fixed and the
   storageprovider ID handling refactored.

   https://github.com/cs3org/reva/pull/3836

*   Bugfix #3867: Restore last version after positive result

   We fixed a bug in the copy routine that prevented restoring of a previous version after
   post-processing (e.g. virus scanning)

   https://github.com/owncloud/enterprise/issues/5709
   https://github.com/cs3org/reva/pull/3867

*   Bugfix #3849: Prevent sharing space roots and personal spaces

   We fixed a problem where sharing space roots or adding members to a personal space was possible.

   https://github.com/cs3org/reva/pull/3849

*   Enhancement #3865: Remove unneccessary code from gateway

   Delete unused removeReference code from gateway

   https://github.com/cs3org/reva/pull/3865

*   Enhancement #3895: Add missing expiry date to shares

   We have added expiry dates to the shares

   https://github.com/owncloud/ocis/issues/5442
   https://github.com/cs3org/reva/pull/3895

