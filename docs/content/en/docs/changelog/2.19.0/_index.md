
---
title: "v2.19.0"
linkTitle: "v2.19.0"
weight: 40
description: >
  Changelog for Reva v2.19.0 (2024-02-22)
---

Changelog for reva 2.19.0 (2024-02-22)
=======================================

The following sections list the changes in reva 2.19.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4464: Do not lose revisions when restoring the first revision
*   Fix #4516: The sharemanager can now reject grants with resharing permissions
*   Fix #4512: Bump dependencies
*   Fix #4481: Distinguish failure and node metadata reversal
*   Fix #4456: Do not lose revisions when restoring the first revision
*   Fix #4472: Fix concurrent access to a map
*   Fix #4457: Fix concurrent map access in sharecache
*   Fix #4498: Fix Content-Disposition header in dav
*   Fix #4461: CORS handling for WebDAV requests fixed
*   Fix #4462: Prevent setting container specific permissions on files
*   Fix #4479: Fix creating documents in the approvider
*   Fix #4474: Make /dav/meta consistent
*   Fix #4446: Disallow to delete a file during the processing
*   Fix #4517: Fix duplicated items in the sharejail root
*   Fix #4473: Decomposedfs now correctly lists sessions
*   Fix #4528: Respect IfNotExist option when uploading in cs3 metadata storage
*   Fix #4503: Fix an error when move
*   Fix #4466: Fix natsjskv store
*   Fix #4533: Fix recursive trashcan purge
*   Fix #4492: Fix the resource name
*   Fix #4463: Fix the resource name
*   Fix #4448: Fix truncating existing files
*   Fix #4434: Fix the upload postprocessing
*   Fix #4469: Handle interrupted uploads
*   Fix #4532: Jsoncs3 cache fixes
*   Fix #4449: Keep failed processing status
*   Fix #4529: We aligned some OCS return codes with oc10
*   Fix #4507: Make tusd CORS headers configurable
*   Fix #4452: More efficient share jail
*   Fix #4476: No need to unmark postprocessing when it was not started
*   Fix #4454: Skip unnecessary share retrieval
*   Fix #4527: Unify datagateway method handling
*   Fix #4530: Drop unnecessary grant exists check
*   Fix #4475: Upload session specific processing flag
*   Enh #4501: Allow sending multiple user ids in one sse event
*   Enh #4485: Modify the concurrency default
*   Enh #4526: Configurable s3 put options
*   Enh #4453: Disable the password policy
*   Enh #4477: Extend ResumePostprocessing event
*   Enh #4491: Add filename incrementor for secret filedrops
*   Enh #4490: Lazy initialize public share manager
*   Enh #4494: Start implementation of a plain posix storage driver
*   Enh #4502: Add spaceindex.AddAll()

Details
-------

*   Bugfix #4464: Do not lose revisions when restoring the first revision

   We no longer prevent modifying grants on locked resources.

   https://github.com/cs3org/reva/pull/4464

*   Bugfix #4516: The sharemanager can now reject grants with resharing permissions

   When disabling resharing we also need to prevent grants from allowing any grant permissions.

   https://github.com/cs3org/reva/pull/4516

*   Bugfix #4512: Bump dependencies

   We updated the dependencies to at least match the ones used in ocis.

   https://github.com/cs3org/reva/pull/4512

*   Bugfix #4481: Distinguish failure and node metadata reversal

   When the final blob move fails we must not remove the node metadata to be able to restart the
   postprocessing process.

   https://github.com/cs3org/reva/pull/4481

*   Bugfix #4456: Do not lose revisions when restoring the first revision

   We fixed a problem where restoring the very first version of a file could delete the current
   version.

   https://github.com/cs3org/reva/pull/4456

*   Bugfix #4472: Fix concurrent access to a map

   We fixed the race condition that led to concurrent map access in a publicshare manager.

   https://github.com/owncloud/ocis/issues/8255
   https://github.com/cs3org/reva/pull/4472

*   Bugfix #4457: Fix concurrent map access in sharecache

   We fixed a problem where the sharecache map would sometimes cause a panic when being accessed
   concurrently.

   https://github.com/cs3org/reva/pull/4457

*   Bugfix #4498: Fix Content-Disposition header in dav

   We have added missing quotes to the Content-Disposition header in the dav service. This fixes
   an issue with files containing special characters in their names.

   https://github.com/owncloud/ocis/issues/8361
   https://github.com/cs3org/reva/pull/4498

*   Bugfix #4461: CORS handling for WebDAV requests fixed

   We now correctly handle CORS headers for WebDAV requests.

   https://github.com/owncloud/ocis/issues/8231
   https://github.com/cs3org/reva/pull/4461

*   Bugfix #4462: Prevent setting container specific permissions on files

   It was possible to set the 'CreateContainer', 'Move' or 'Delete' permissions on file
   resources with a CreateShare request. These permissions are meant to be only set on container
   resources. The UpdateShare request already has a similar check.

   https://github.com/owncloud/ocis/issues/8131
   https://github.com/cs3org/reva/pull/4462

*   Bugfix #4479: Fix creating documents in the approvider

   We fixed a problem with the approvider where an error was reported to the user even though the
   file was created properly.

   https://github.com/cs3org/reva/pull/4479

*   Bugfix #4474: Make /dav/meta consistent

   We now also return absolute paths for shares in the share jail in the /dav/meta endpoint.

   https://github.com/cs3org/reva/pull/4474

*   Bugfix #4446: Disallow to delete a file during the processing

   We want to disallow deleting a file during the processing to prevent collecting the orphan
   uploads.

   https://github.com/cs3org/reva/pull/4446

*   Bugfix #4517: Fix duplicated items in the sharejail root

   We fixed a bug, that caused duplicate items to listed in the sharejail, when a user received
   multiple shares for the same resource.

   https://github.com/owncloud/ocis/issues/8080
   https://github.com/cs3org/reva/pull/4517

*   Bugfix #4473: Decomposedfs now correctly lists sessions

   https://github.com/cs3org/reva/pull/4473

*   Bugfix #4528: Respect IfNotExist option when uploading in cs3 metadata storage

   https://github.com/cs3org/reva/pull/4528

*   Bugfix #4503: Fix an error when move

   We fixed a bug that caused Internal Server Error when move using destination id

   https://github.com/owncloud/ocis/issues/6739
   https://github.com/cs3org/reva/pull/4503

*   Bugfix #4466: Fix natsjskv store

   Small mistake in last iteration made authorization not working.

   https://github.com/cs3org/reva/pull/4466

*   Bugfix #4533: Fix recursive trashcan purge

   We have fixed a bug in the trashcan purge process that did not delete folder structures
   recursively.

   https://github.com/owncloud/ocis/issues/8473
   https://github.com/cs3org/reva/pull/4533

*   Bugfix #4492: Fix the resource name

   We fixed a problem where after renaming resource as sharer the receiver see a new name for dav and
   wedav endpoints.

   https://github.com/owncloud/ocis/issues/8242
   https://github.com/cs3org/reva/pull/4492

*   Bugfix #4463: Fix the resource name

   We fixed a problem where after renaming resource as sharer the receiver see a new name.

   https://github.com/owncloud/ocis/issues/8242
   https://github.com/cs3org/reva/pull/4463

*   Bugfix #4448: Fix truncating existing files

   We fixed a problem where existing files kept their content when being overwritten by a 0-byte
   file.

   https://github.com/cs3org/reva/pull/4448

*   Bugfix #4434: Fix the upload postprocessing

   We fixed the upload postprocessing when the destination file does not exist anymore.

   https://github.com/cs3org/reva/pull/4434

*   Bugfix #4469: Handle interrupted uploads

   We fixed a bug where interrupted uploads were not discarded properly.

   https://github.com/cs3org/reva/pull/4469

*   Bugfix #4532: Jsoncs3 cache fixes

   The jsoncs3 share manager now retries persisting if the file already existed and picks up the
   etag of the upload response in all cases.

   https://github.com/cs3org/reva/pull/4532

*   Bugfix #4449: Keep failed processing status

   We now keep tho postprocessing status when a blob could not be copied to the blobstore.

   https://github.com/cs3org/reva/pull/4449

*   Bugfix #4529: We aligned some OCS return codes with oc10

   https://github.com/owncloud/ocis/issues/1233
   https://github.com/cs3org/reva/pull/4529

*   Bugfix #4507: Make tusd CORS headers configurable

   We bumped tusd to 1.13.0 and made CORS headers configurable via mapstructure.

   https://github.com/cs3org/reva/pull/4507

*   Bugfix #4452: More efficient share jail

   The share jail was stating every shared recource twice when listing the share jail root. For no
   good reason. And it was not sending filters when it could.

   https://github.com/cs3org/reva/pull/4452

*   Bugfix #4476: No need to unmark postprocessing when it was not started

   https://github.com/cs3org/reva/pull/4476

*   Bugfix #4454: Skip unnecessary share retrieval

   https://github.com/cs3org/reva/pull/4454

*   Bugfix #4527: Unify datagateway method handling

   The datagateway now unpacks and forwards all HTTP methods

   https://github.com/cs3org/reva/pull/4527

*   Bugfix #4530: Drop unnecessary grant exists check

   At least the jsoncs3 share manager properly returns an ALREADY_EXISTS response when trying to
   add a share to a resource that has already been shared with the grantee.

   https://github.com/cs3org/reva/pull/4530

*   Bugfix #4475: Upload session specific processing flag

   To make every upload session have a dedicated processing status, upload sessions are now
   treated as in processing when all bytes have been received instead of checking the node
   metadata.

   https://github.com/cs3org/reva/pull/4475

*   Enhancement #4501: Allow sending multiple user ids in one sse event

   Sending multiple user ids in one sse event is now possible which reduces the number of sent
   events.

   https://github.com/cs3org/reva/pull/4501

*   Enhancement #4485: Modify the concurrency default

   We have changed the default MaxConcurrency value from 100 to 5 to prevent too frequent gc runs on
   low memory systems.

   https://github.com/owncloud/ocis/issues/8257
   https://github.com/cs3org/reva/pull/4485

*   Enhancement #4526: Configurable s3 put options

   The s3ng blobstore can now be configured with several options:
   `s3.disable_content_sha254`, `s3.disable_multipart`, `s3.send_content_md5`,
   `s3.concurrent_stream_parts`, `s3.num_threads` and `s3.part_size`. If unset we default
   to `s3.send_content_md5: true`, which was hardcoded before. We also default to
   `s3.concurrent_stream_parts: true` and `s3.num_threads: 4` to allow concurrent uploads
   even when `s3.send_content_md5` is set to `true`. When tweaking the uploads try setting
   `s3.send_content_md5: false` and `s3.concurrent_stream_parts: false` first, as this will
   try to concurrently stream an uploaded file to the s3 store without cutting it into parts first.

   https://github.com/cs3org/reva/pull/4526

*   Enhancement #4453: Disable the password policy

   We reworked and moved disabling the password policy logic to the ocis.

   https://github.com/owncloud/ocis/issues/7916
   https://github.com/cs3org/reva/pull/4453

*   Enhancement #4477: Extend ResumePostprocessing event

   Instead of just sending an uploadID, one can set a postprocessing step now to restart all
   uploads in this step Also adds a new postprocessing step - "finished" - which means that
   postprocessing is finished but the storage provider hasn't acknowledged it yet.

   https://github.com/cs3org/reva/pull/4477

*   Enhancement #4491: Add filename incrementor for secret filedrops

   We have added a function that appends a number to the filename if the file already exists in a
   secret filedrop. This is useful if you want to upload a file with the same name multiple times.

   https://github.com/owncloud/ocis/issues/8291
   https://github.com/cs3org/reva/pull/4491

*   Enhancement #4490: Lazy initialize public share manager

   Unlike the share manager the public share manager was initializing its data structure on
   startup. This can lead to failed ocis starts (in single binary case) or to restarting `sharing`
   pods when running in containerized environment.

   https://github.com/cs3org/reva/pull/4490

*   Enhancement #4494: Start implementation of a plain posix storage driver

   We started to lay the groundwork for a new posixfs storage driver based on decomposedfs. We also
   refactored decomposedfs to be a bit more modular and cleaned up the initialization.

   https://github.com/cs3org/reva/pull/4494

*   Enhancement #4502: Add spaceindex.AddAll()

   We now expose an AddAll() function that allows adding multiple entries to a space index with a
   single lock.

   https://github.com/cs3org/reva/pull/4502

