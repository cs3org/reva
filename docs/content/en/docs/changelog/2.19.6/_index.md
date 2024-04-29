
---
title: "v2.19.6"
linkTitle: "v2.19.6"
weight: 40
description: >
  Changelog for Reva v2.19.6 (2024-04-29)
---

Changelog for reva 2.19.6 (2024-04-29)
=======================================

The following sections list the changes in reva 2.19.6 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4654: Write blob based on session id
*   Fix #4666: Fix uploading via a public link
*   Fix #4665: Fix creating documents in nested folders of public shares
*   Enh #4655: Bump mockery to v2.40.2
*   Enh #4664: Add ScanData to Uploadsession

Details
-------

*   Bugfix #4654: Write blob based on session id

   Decomposedfs now uses the session id and size when moving an uplode to the blobstore. This fixes
   a cornercase that prevents an upload session from correctly being finished when another
   upload session to the file was started and already finished.

   https://github.com/cs3org/reva/pull/4654
   https://github.com/cs3org/reva/pull/4615

*   Bugfix #4666: Fix uploading via a public link

   Fix http error when uploading via a public link

   https://github.com/owncloud/ocis/issues/8699
   https://github.com/cs3org/reva/pull/4666
   https://github.com/cs3org/reva/pull/4589

*   Bugfix #4665: Fix creating documents in nested folders of public shares

   We fixed a bug that prevented creating new documented in a nested folder of a public share.

   https://github.com/owncloud/ocis/issues/8957
   https://github.com/cs3org/reva/pull/4665
   https://github.com/cs3org/reva/pull/4660

*   Enhancement #4655: Bump mockery to v2.40.2

   We switched to the latest mockery and changed to .mockery.yaml based mock generation.

   https://github.com/cs3org/reva/pull/4655
   https://github.com/cs3org/reva/pull/4614

*   Enhancement #4664: Add ScanData to Uploadsession

   Adds virus scan results to the upload session.

   https://github.com/cs3org/reva/pull/4664
   https://github.com/cs3org/reva/pull/4657

