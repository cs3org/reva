
---
title: "v1.18.0"
linkTitle: "v1.18.0"
weight: 40
description: >
  Changelog for Reva v1.18.0 (2022-02-11)
---

Changelog for reva 1.18.0 (2022-02-11)
=======================================

The following sections list the changes in reva 1.18.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2370: Fixes for apps in public shares, project spaces for EOS driver
 * Fix #2374: Fix webdav copy of zero byte files
 * Fix #2478: Use ocs permission objects in the reva GRPC client
 * Fix #2368: Return wrapped paths for recycled items in storage provider
 * Chg #2354: Return not found when updating non existent space
 * Enh #1209: Reva CephFS module v0.2.1
 * Enh #2341: Use CS3 permissions API
 * Enh #2350: Add file locking methods to the storage and filesystem interfaces
 * Enh #2379: Add new file url of the app provider to the ocs capabilities
 * Enh #2369: Implement TouchFile from the CS3apis
 * Enh #2385: Allow to create new files with the app provider on public links
 * Enh #2397: Product field in OCS version
 * Enh #2393: Update tus/tusd to version 1.8.0
 * Enh #2205: Modify group and user managers to skip fetching specified metadata
 * Enh #2232: Make ocs resource info cache interoperable across drivers
 * Enh #2233: Populate owner data in the ocs and ocdav services
 * Enh #2278: OIDC driver changes for lightweight users

Details
-------

 * Bugfix #2370: Fixes for apps in public shares, project spaces for EOS driver

   https://github.com/cs3org/reva/pull/2370

 * Bugfix #2374: Fix webdav copy of zero byte files

   We've fixed the webdav copy action of zero byte files, which was not performed because the
   webdav api assumed, that zero byte uploads are created when initiating the upload, which was
   recently removed from all storage drivers. Therefore the webdav api also uploads zero byte
   files after initiating the upload.

   https://github.com/cs3org/reva/pull/2374
   https://github.com/cs3org/reva/pull/2309

 * Bugfix #2478: Use ocs permission objects in the reva GRPC client

   There was a bug introduced by differing CS3APIs permission definitions for the same role
   across services. This is a first step in making all services use consistent definitions.

   https://github.com/cs3org/reva/pull/2478

 * Bugfix #2368: Return wrapped paths for recycled items in storage provider

   https://github.com/cs3org/reva/pull/2368

 * Change #2354: Return not found when updating non existent space

   If a spaceid of a space which is updated doesn't exist, handle it as a not found error.

   https://github.com/cs3org/reva/pull/2354

 * Enhancement #1209: Reva CephFS module v0.2.1

   https://github.com/cs3org/reva/pull/1209

 * Enhancement #2341: Use CS3 permissions API

   Added calls to the CS3 permissions API to the decomposedfs in order to check the user
   permissions.

   https://github.com/cs3org/reva/pull/2341

 * Enhancement #2350: Add file locking methods to the storage and filesystem interfaces

   We've added the file locking methods from the CS3apis to the storage and filesystem
   interfaces. As of now they are dummy implementations and will only return "unimplemented"
   errors.

   https://github.com/cs3org/reva/pull/2350
   https://github.com/cs3org/cs3apis/pull/160

 * Enhancement #2379: Add new file url of the app provider to the ocs capabilities

   We've added the new file capability of the app provider to the ocs capabilities, so that clients
   can discover this url analogous to the app list and file open urls.

   https://github.com/cs3org/reva/pull/2379
   https://github.com/owncloud/ocis/pull/2884
   https://github.com/owncloud/web/pull/5890#issuecomment-993905242

 * Enhancement #2369: Implement TouchFile from the CS3apis

   We've updated the CS3apis and implemented the TouchFile method.

   https://github.com/cs3org/reva/pull/2369
   https://github.com/cs3org/cs3apis/pull/154

 * Enhancement #2385: Allow to create new files with the app provider on public links

   We've added the option to create files with the app provider on public links.

   https://github.com/cs3org/reva/pull/2385

 * Enhancement #2397: Product field in OCS version

   We've added a new field to the OCS Version, which is supposed to announce the product name. The
   web ui as a client will make use of it to make the backend product and version available (e.g. for
   easier bug reports).

   https://github.com/cs3org/reva/pull/2397

 * Enhancement #2393: Update tus/tusd to version 1.8.0

   We've update tus/tusd to version 1.8.0.

   https://github.com/cs3org/reva/issues/2393
   https://github.com/cs3org/reva/pull/2224

 * Enhancement #2205: Modify group and user managers to skip fetching specified metadata

   https://github.com/cs3org/reva/pull/2205

 * Enhancement #2232: Make ocs resource info cache interoperable across drivers

   https://github.com/cs3org/reva/pull/2232

 * Enhancement #2233: Populate owner data in the ocs and ocdav services

   https://github.com/cs3org/reva/pull/2233

 * Enhancement #2278: OIDC driver changes for lightweight users

   https://github.com/cs3org/reva/pull/2278


