
---
title: "v2.27.0"
linkTitle: "v2.27.0"
weight: 40
description: >
  Changelog for Reva v2.27.0 (2024-12-12)
---

Changelog for reva 2.27.0 (2024-12-12)
=======================================

The following sections list the changes in reva 2.27.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4985: Drop unneeded session locks
*   Fix #5000: Fix ceph build
*   Fix #4989: Deleting OCM share also updates storageprovider
*   Enh #4998: Emit event when an ocm share is received
*   Enh #4996: Get rid of some cases of unstructured logging

Details
-------

*   Bugfix #4985: Drop unneeded session locks

   We no longer lock session metadada files, as they are already written atomically.

   https://github.com/cs3org/reva/pull/4985

*   Bugfix #5000: Fix ceph build

   https://github.com/cs3org/reva/pull/5000

*   Bugfix #4989: Deleting OCM share also updates storageprovider

   When remvoving an OCM share we're now also removing the related grant from the storage
   provider.

   https://github.com/owncloud/ocis/issues/10262
   https://github.com/cs3org/reva/pull/4989

*   Enhancement #4998: Emit event when an ocm share is received

   https://github.com/owncloud/ocis/issues/10718
   https://github.com/cs3org/reva/pull/4998

*   Enhancement #4996: Get rid of some cases of unstructured logging

   https://github.com/cs3org/reva/pull/4996

