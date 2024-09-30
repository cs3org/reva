
---
title: "v2.25.0"
linkTitle: "v2.25.0"
weight: 40
description: >
  Changelog for Reva v2.25.0 (2024-09-30)
---

Changelog for reva 2.25.0 (2024-09-30)
=======================================

The following sections list the changes in reva 2.25.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4854: Added ShareUpdate activity
*   Fix #4865: Better response codes for app new endpoint
*   Fix #4858: Better response codes for app new endpoint
*   Fix #4867: Fix remaining space calculation for S3 blobstore
*   Fix #4852: Populate public link user correctly
*   Fix #4859: Fixed the collaboration service registration
*   Fix #4835: Fix sharejail stat id
*   Fix #4856: Fix time conversion
*   Fix #4851: Use gateway selector in sciencemesh
*   Fix #4850: Write upload session info atomically
*   Enh #4866: Unit test the json ocm invite manager
*   Enh #4847: Add IsVersion to UploadReadyEvent
*   Enh #4868: Improve metadata client errors
*   Enh #4848: Add trashbin support to posixfs alongside other improvements

Details
-------

*   Bugfix #4854: Added ShareUpdate activity

   Added the ShareUpdate activity in the space context.

   https://github.com/owncloud/ocis/issues/10011
   https://github.com/cs3org/reva/pull/4854

*   Bugfix #4865: Better response codes for app new endpoint

   We fixed the response codes for the app new endpoint. Permission denied is now backing the
   request.

   https://github.com/cs3org/reva/pull/4865

*   Bugfix #4858: Better response codes for app new endpoint

   We fixed the response codes for the app new endpoint.

   https://github.com/cs3org/reva/pull/4858

*   Bugfix #4867: Fix remaining space calculation for S3 blobstore

   The calculation of the remaining space in the S3 blobstore was incorrectly using the remaining
   space of the local disk instead.

   https://github.com/cs3org/reva/pull/4867

*   Bugfix #4852: Populate public link user correctly

   When authenticating via public link, always add the `public` user instead of the link owner

   https://github.com/cs3org/reva/pull/4852

*   Bugfix #4859: Fixed the collaboration service registration

   Fixed an issue when the collaboration service registers apps also for binary and unknown mime
   types.

   https://github.com/owncloud/ocis/issues/10086
   https://github.com/cs3org/reva/pull/4859

*   Bugfix #4835: Fix sharejail stat id

   Stating a share jail mountpoint now returns the same resourceid as in the directory listing of
   the share jail root.

   https://github.com/owncloud/ocis/issues/9933
   https://github.com/cs3org/reva/pull/4835

*   Bugfix #4856: Fix time conversion

   We fixed a nil pointer in a time conversion

   https://github.com/cs3org/reva/pull/4856

*   Bugfix #4851: Use gateway selector in sciencemesh

   We now use a selector to get fresh ip addresses when running ocis in a kubernetes clustern.

   https://github.com/cs3org/reva/pull/4851

*   Bugfix #4850: Write upload session info atomically

   We now use a lock and atomic write on upload session metadata to prevent empty reads. A virus scan
   event might cause the file to be truncated and then a finished event might try to read the file,
   just getting an empty string.

   https://github.com/cs3org/reva/pull/4850

*   Enhancement #4866: Unit test the json ocm invite manager

   We added unit tests for the json ocm invite manager

   https://github.com/cs3org/reva/pull/4866

*   Enhancement #4847: Add IsVersion to UploadReadyEvent

   Adds an IsVersion flag indicating that this upload is a version of an existing file

   https://github.com/cs3org/reva/pull/4847

*   Enhancement #4868: Improve metadata client errors

   We now return a more descripive error message when the metadata client cannot create a space
   because it already exists.

   https://github.com/cs3org/reva/pull/4868

*   Enhancement #4848: Add trashbin support to posixfs alongside other improvements

   We added support for trashbins to posixfs. Posixfs also saw a number of other improvement,
   bugfixes and optimizations.

   https://github.com/cs3org/reva/pull/4848
   https://github.com/cs3org/reva/pull/4779

