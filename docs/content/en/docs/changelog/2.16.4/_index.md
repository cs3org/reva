
---
title: "v2.16.4"
linkTitle: "v2.16.4"
weight: 40
description: >
  Changelog for Reva v2.16.4 (2024-03-22)
---

Changelog for reva 2.16.4 (2024-03-22)
=======================================

The following sections list the changes in reva 2.16.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4398: Fix ceph build
*   Fix #4396: Allow an empty credentials chain in the auth middleware
*   Fix #4423: Fix disconnected traces
*   Fix #4590: Fix uploading via a public link
*   Fix #4470: Keep failed processing status
*   Enh #4397: Introduce UploadSessionLister interface

Details
-------

*   Bugfix #4398: Fix ceph build

   Fix ceph build as already fixed in edge

   https://github.com/cs3org/reva/pull/4398
   https://github.com/cs3org/reva/pull/4147

*   Bugfix #4396: Allow an empty credentials chain in the auth middleware

   When running with ocis, all external http-authentication is handled by the proxy service. So
   the reva auth middleware should not try to do any basic or bearer auth.

   https://github.com/owncloud/ocis/issues/6692
   https://github.com/cs3org/reva/pull/4396
   https://github.com/cs3org/reva/pull/4241

*   Bugfix #4423: Fix disconnected traces

   We fixed a problem where the appctx logger was using a new traceid instead of picking up the one
   from the trace parent.

   https://github.com/cs3org/reva/pull/4423

*   Bugfix #4590: Fix uploading via a public link

   Fix http error when uploading via a public link

   https://github.com/owncloud/ocis/issues/8658
   https://github.com/owncloud/ocis/issues/8629
   https://github.com/cs3org/reva/pull/4590

*   Bugfix #4470: Keep failed processing status

   We now keep the postprocessing status when a blob could not be copied to the blobstore.

   https://github.com/cs3org/reva/pull/4470
   https://github.com/cs3org/reva/pull/4449

*   Enhancement #4397: Introduce UploadSessionLister interface

   We introduced a new UploadSessionLister interface that allows better control of upload
   sessions. Upload sessions include the processing state and can be used to filter and purge the
   list of currently ongoing upload sessions.

   https://github.com/cs3org/reva/pull/4397
   https://github.com/cs3org/reva/pull/4375

