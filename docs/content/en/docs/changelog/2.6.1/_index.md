
---
title: "v2.6.1"
linkTitle: "v2.6.1"
weight: 40
description: >
  Changelog for Reva v2.6.1 (2022-06-27)
---

Changelog for reva 2.6.1 (2022-06-27)
=======================================

The following sections list the changes in reva 2.6.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2998: Fix 0-byte-uploads
 * Enh #3983: Add capability for alias links
 * Enh #3000: Make less stat requests
 * Enh #3003: Distinguish GRPC FAILED_PRECONDITION and ABORTED codes
 * Enh #3005: Remove unused HomeMapping variable

Details
-------

 * Bugfix #2998: Fix 0-byte-uploads

   We fixed a problem with 0-byte uploads by using TouchFile instead of going through TUS
   (decomposedfs and owncloudsql storage drivers only for now).

   https://github.com/cs3org/reva/pull/2998

 * Enhancement #3983: Add capability for alias links

   For better UX clients need a way to discover if alias links are supported by the server. We added a
   capability under "files_sharing/public/alias"

   https://github.com/owncloud/ocis/issues/3983
   https://github.com/cs3org/reva/pull/2991

 * Enhancement #3000: Make less stat requests

   The /dav/spaces endpoint now constructs a reference instead of making a lookup grpc call,
   reducing the number of requests.

   https://github.com/cs3org/reva/pull/3000

 * Enhancement #3003: Distinguish GRPC FAILED_PRECONDITION and ABORTED codes

   Webdav distinguishes between 412 precondition failed for if match errors for locks or etags,
   uses 405 Method Not Allowed when trying to MKCOL an already existing collection and 409
   Conflict when intermediate collections are missing.

   The CS3 GRPC status codes are modeled after
   https://github.com/googleapis/googleapis/blob/master/google/rpc/code.proto. When
   trying to use the error codes to distinguish these cases on a storageprovider CreateDir call we
   can map ALREADY_EXISTS to 405, FAILED_PRECONDITION to 409 and ABORTED to 412.

   Unfortunately, we currently use and map FAILED_PRECONDITION to 412. I assume, because the
   naming is very similar to PreconditionFailed. However the GRPC docs are very clear that
   ABORTED should be used, specifically mentioning etags and locks.

   With this PR we internally clean up the usage in the decomposedfs and mapping in the ocdav
   handler.

   https://github.com/cs3org/reva/pull/3003
   https://github.com/cs3org/reva/pull/3010

 * Enhancement #3005: Remove unused HomeMapping variable

   We have removed the unused HomeMapping variable from the gateway.

   https://github.com/cs3org/reva/pull/3005


