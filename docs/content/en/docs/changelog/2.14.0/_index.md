
---
title: "v2.14.0"
linkTitle: "v2.14.0"
weight: 40
description: >
  Changelog for Reva v2.14.0 (2023-06-05)
---

Changelog for reva 2.14.0 (2023-06-05)
=======================================

The following sections list the changes in reva 2.14.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3919: We added missing timestamps to events
*   Fix #3911: Clean IDCache properly
*   Fix #3896: Do not lose old revisions when overwriting a file during copy
*   Fix #3918: Dont enumerate users
*   Fix #3902: Do not try to use the cache for empty node
*   Fix #3877: Empty exact list while searching for a sharee
*   Fix #3906: Fix preflight requests
*   Fix #3934: Fix the space editor permissions
*   Fix #3899: Harden uploads
*   Fix #3917: Prevent last space manager from leaving
*   Fix #3866: Fix public link lookup performance
*   Fix #3904: Improve performance of directory listings
*   Enh #3893: Cleanup Space Delete permissions
*   Enh #3894: Fix err when the user share the locked file
*   Enh #3913: Introduce FullTextSearch Capability
*   Enh #3898: Add Graph User capabilities
*   Enh #3496: Add otlp tracing exporter
*   Enh #3922: Rename permissions

Details
-------

*   Bugfix #3919: We added missing timestamps to events

   We added missing timestamps to events

   https://github.com/owncloud/ocis/issues/3753
   https://github.com/cs3org/reva/pull/3919

*   Bugfix #3911: Clean IDCache properly

   Decomposedfs' subpackage `tree` uses an idCache to avoid reading too often from disc. In case
   of a `move` or `delete` this cache was properly cleaned, but when renaming a file (= move with
   same parent) the cache wasn't cleaned. This lead to strange behaviour when uploading files
   with the same name and renaming them

   https://github.com/cs3org/reva/pull/3911
   https://github.com/cs3org/reva/pull/3903

*   Bugfix #3896: Do not lose old revisions when overwriting a file during copy

   We no longer delete-and-upload targets of copy operations but rather add a new version with the
   source content.

   This makes "overwrite when copying" behave the same as "overwrite when uploading".

   Overwriting when moving a file still deletes the old file (moves it into the trash) and replaces
   the whole file including the revisions of the source file.

   https://github.com/cs3org/reva/pull/3896

*   Bugfix #3918: Dont enumerate users

   Fixes a user enumeration via DELETE share endpoint

   https://github.com/cs3org/reva/pull/3918
   https://github.com/cs3org/reva/pull/3916

*   Bugfix #3902: Do not try to use the cache for empty node

   We fixed a problem where nodes that did not have an ID set were still trying to use the cache for
   their metadata resulting in clashing cache keys.

   https://github.com/cs3org/reva/pull/3902

*   Bugfix #3877: Empty exact list while searching for a sharee

   We fixed a bug in the sharing api, it always returns an empty exact list while searching for a
   sharee

   https://github.com/owncloud/ocis/issues/4265
   https://github.com/cs3org/reva/pull/3877

*   Bugfix #3906: Fix preflight requests

   The datagateway now correctly overwrites the preconfigured CORS related headers with the
   headers returned by a dataprovider.

   https://github.com/cs3org/reva/pull/3906

*   Bugfix #3934: Fix the space editor permissions

   We fixed the permissions of a space editor which accidentally granted the permission to purge
   the trash bin.

   https://github.com/cs3org/reva/pull/3934

*   Bugfix #3899: Harden uploads

   Uploads now check response headers for a file id and omit a subsequent stat request which might
   land on a storage provider that does not yet see the new file due to latency, eg. when NFS caches
   direntries.

   https://github.com/cs3org/reva/pull/3899

*   Bugfix #3917: Prevent last space manager from leaving

   It should not be possible for the last remaining space manager to change his role or get changed
   by others.

   https://github.com/cs3org/reva/pull/3917

*   Bugfix #3866: Fix public link lookup performance

   Fix inefficient path based space lookup for public links

   https://github.com/cs3org/reva/pull/3866

*   Bugfix #3904: Improve performance of directory listings

   We improved the performance of directory listing by rendering the propfind XML concurrently.

   https://github.com/cs3org/reva/pull/3904

*   Enhancement #3893: Cleanup Space Delete permissions

   Space Delete and Disable permissions ("Drive.ReadWriteEnabled", "delete-all-spaces",
   "delete-all-home-spaces") were overlapping and not clear differentiatable. The new logic
   is as follows: - "Drive.ReadWriteEnabled" allows enabling or disabling a project space -
   "delete-all-home-spaces" allows deleting personal spaces of users - "delete-all-spaces"
   allows deleting a project space - Space Mangers can still disable/enable a drive

   https://github.com/cs3org/reva/pull/3893

*   Enhancement #3894: Fix err when the user share the locked file

   Fix unexpected behavior when the user try to share the locked file

   https://github.com/owncloud/ocis/issues/6197
   https://github.com/cs3org/reva/pull/3894

*   Enhancement #3913: Introduce FullTextSearch Capability

   Add a capability that shows if fulltextsearch is supported by the server

   https://github.com/cs3org/reva/pull/3913

*   Enhancement #3898: Add Graph User capabilities

   Add capabilities to show if user can be created or deleted and if they can change their password
   on self service

   https://github.com/cs3org/reva/pull/3898

*   Enhancement #3496: Add otlp tracing exporter

   We can now use `tracing_exporter=otlp` to send traces using the otlp exporter.

   https://github.com/cs3org/reva/pull/3496

*   Enhancement #3922: Rename permissions

   Rename permissions to be consistent and future proof

   https://github.com/cs3org/reva/pull/3922

