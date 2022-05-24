
---
title: "v2.4.0"
linkTitle: "v2.4.0"
weight: 40
description: >
  Changelog for Reva v2.4.0 (2022-05-24)
---

Changelog for reva 2.4.0 (2022-05-24)
=======================================

The following sections list the changes in reva 2.4.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2854: Handle non uuid space and nodeid in decomposedfs
 * Fix #2853: Filter CS3 share manager listing
 * Fix #2868: Actually remove blobs when purging
 * Fix #2882: Fix FileUploaded event being emitted too early
 * Fix #2848: Fix storage id in the references in the ItemTrashed events
 * Fix #2852: Fix rcbox dependency on reva 1.18
 * Fix #3505: Fix creating a new file with wopi
 * Fix #2885: Move stat out of usershareprovider
 * Fix #2883: Fix role consideration when updating a share
 * Fix #2864: Fix Grant Space IDs
 * Fix #2870: Update quota calculation
 * Fix #2876: Fix version number in status page
 * Fix #2829: Don't include versions in quota
 * Chg #2856: Do not allow to edit disabled spaces
 * Enh #3741: Add download endpoint to ocdav versions API
 * Enh #2884: Show mounted shares in virtual share jail root
 * Enh #2792: Use storageproviderid for spaces routing

Details
-------

 * Bugfix #2854: Handle non uuid space and nodeid in decomposedfs

   The decomposedfs no longer panics when trying to look up spaces with a non uuid length id.

   https://github.com/cs3org/reva/pull/2854

 * Bugfix #2853: Filter CS3 share manager listing

   The cs3 share manager driver now correctly filters user and group queries

   https://github.com/cs3org/reva/pull/2853

 * Bugfix #2868: Actually remove blobs when purging

   Blobs were not being deleted properly on purge. Now if a folder gets purged all its children will
   be deleted

   https://github.com/cs3org/reva/pull/2868

 * Bugfix #2882: Fix FileUploaded event being emitted too early

   We fixed a problem where the FileUploaded event was emitted before the upload had actually
   finished.

   https://github.com/cs3org/reva/pull/2882

 * Bugfix #2848: Fix storage id in the references in the ItemTrashed events

   https://github.com/cs3org/reva/pull/2848

 * Bugfix #2852: Fix rcbox dependency on reva 1.18

   The cbox package no longer depends on reva 1.18.

   https://github.com/cs3org/reva/pull/2852

 * Bugfix #3505: Fix creating a new file with wopi

   Fixed a bug in the appprovider which prevented creating new files.

   https://github.com/owncloud/ocis/issues/3505
   https://github.com/cs3org/reva/pull/2869

 * Bugfix #2885: Move stat out of usershareprovider

   The sharesstorageprovider now only stats the acceptet shares when necessary.

   https://github.com/cs3org/reva/pull/2885

 * Bugfix #2883: Fix role consideration when updating a share

   Previously when updating a share the endpoint only considered the permissions, now this also
   respects a given role.

   https://github.com/cs3org/reva/pull/2883

 * Bugfix #2864: Fix Grant Space IDs

   The opaqueID for a grant space was incorrectly overwritten with the root space id.

   https://github.com/cs3org/reva/pull/2864

 * Bugfix #2870: Update quota calculation

   We now render the `free` and `definition` quota properties, taking into account the remaining
   bytes reported from the storage space and calculating `relative` only when possible.

   https://github.com/cs3org/reva/pull/2870

 * Bugfix #2876: Fix version number in status page

   We needed to undo the version number changes on the status page to keep compatibility for legacy
   clients. We added a new field `productversion` for the actual version of the product.

   https://github.com/cs3org/reva/pull/2876
   https://github.com/cs3org/reva/pull/2889

 * Bugfix #2829: Don't include versions in quota

   Fixed the quota check to not count the quota of previous versions.

   https://github.com/owncloud/ocis/issues/2829
   https://github.com/cs3org/reva/pull/2863

 * Change #2856: Do not allow to edit disabled spaces

   Previously managers could still upload to disabled spaces. This is now forbidden

   https://github.com/cs3org/reva/pull/2856

 * Enhancement #3741: Add download endpoint to ocdav versions API

   Added missing endpoints to the ocdav versions API. This enables downloads of previous file
   versions.

   https://github.com/owncloud/ocis/issues/3741
   https://github.com/cs3org/reva/pull/2855

 * Enhancement #2884: Show mounted shares in virtual share jail root

   The virtual share jail now shows the mounted shares to allow the desktop client to sync that
   collection.

   https://github.com/owncloud/ocis/issues/3719
   https://github.com/cs3org/reva/pull/2884

 * Enhancement #2792: Use storageproviderid for spaces routing

   We made the spaces registry aware of storageprovider ids and use them to route directly to the
   correct storageprovider

   https://github.com/cs3org/reva/pull/2792


