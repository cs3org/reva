
---
title: "v2.16.0"
linkTitle: "v2.16.0"
weight: 40
description: >
  Changelog for Reva v2.16.0 (2023-08-21)
---

Changelog for reva 2.16.0 (2023-08-21)
=======================================

The following sections list the changes in reva 2.16.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4051: Set treesize when creating a storage space
*   Fix #4093: Fix the error handling
*   Fix #4111: Return already exists error when child already exists
*   Fix #4086: Fix ocs status code for not enough permission response
*   Fix #4101: Make the jsoncs3 share manager indexes more robust
*   Fix #4099: Fix logging upload errors
*   Fix #4078: Fix the default document language for OnlyOffice
*   Fix #4082: Fix propfind permissions
*   Fix #4100: S3ng include md5 checksum on put
*   Fix #4096: Fix the user shares list
*   Fix #4076: Fix WebDAV permissions for space managers
*   Fix #4117: Fix jsoncs3 atomic persistence
*   Fix #4081: Propagate sizeDiff
*   Fix #4091: Register WebDAV HTTP methods with chi
*   Fix #4107: Return lock when requested
*   Fix #4075: Revert 4065 - bypass proxy on upload
*   Enh #4089: Async propagation (experimental)
*   Enh #4074: Allow configuring the max size of grpc messages
*   Enh #4083: Allow for rolling back migrations
*   Enh #4014: En-/Disable DEPTH:inifinity in PROPFIND
*   Enh #4072: Allow to specify a shutdown timeout
*   Enh #4103: Add .oform mimetype
*   Enh #4098: Allow naming nats connections
*   Enh #4085: Add registry refresh
*   Enh #4097: Remove app ticker logs
*   Enh #4090: Add Capability for sse
*   Enh #4110: Tracing events propgation

Details
-------

*   Bugfix #4051: Set treesize when creating a storage space

   We now explicitly set the treesize metadata to zero when creating a new storage space. This
   prevents empty treesize values for spaces with out any data.

   https://github.com/cs3org/reva/pull/4051

*   Bugfix #4093: Fix the error handling

   Fix the error handling and prevent the nil pointer error

   https://github.com/owncloud/ocis/issues/6929
   https://github.com/cs3org/reva/pull/4093

*   Bugfix #4111: Return already exists error when child already exists

   Prevents two concurrent requests from creating the same file

   https://github.com/cs3org/reva/pull/4111

*   Bugfix #4086: Fix ocs status code for not enough permission response

   Request to re-share a resource or update a share by a user who does not have enough permission on
   the resource returned a 404 status code. This is fixed and a 403 status code is returned instead.

   https://github.com/owncloud/ocis/issues/6670
   https://github.com/cs3org/reva/pull/4086

*   Bugfix #4101: Make the jsoncs3 share manager indexes more robust

   We fixed a problem where the jsoncs3 share manager indexes could get out of sync.

   https://github.com/cs3org/reva/pull/4101

*   Bugfix #4099: Fix logging upload errors

   We fixed a problem where problems with uploading blobs to the blobstore weren't logged.

   https://github.com/cs3org/reva/pull/4099

*   Bugfix #4078: Fix the default document language for OnlyOffice

   Fix the default document language for OnlyOffice

   https://github.com/owncloud/enterprise/issues/5807
   https://github.com/cs3org/reva/pull/4078

*   Bugfix #4082: Fix propfind permissions

   Propfinds permissions field would always contain the permissions of the requested resource,
   even for its children This is fixed.

   https://github.com/cs3org/reva/pull/4082

*   Bugfix #4100: S3ng include md5 checksum on put

   We've fixed the S3 put operation of the S3ng storage to include a md5 checksum.

   This md5 checksum is needed when a bucket has a retention period configured (see
   https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html).

   https://github.com/cs3org/reva/pull/4100

*   Bugfix #4096: Fix the user shares list

   Filter out a share if ShareWith is not found because the user or group already deleted

   https://github.com/owncloud/ocis/issues/6730
   https://github.com/cs3org/reva/pull/4096

*   Bugfix #4076: Fix WebDAV permissions for space managers

   Sub shares of a space were shown as incoming shares for space manager incorrectly.

   https://github.com/cs3org/reva/pull/4076

*   Bugfix #4117: Fix jsoncs3 atomic persistence

   The jsoncs3 share manager now uses etags instead of mtimes to determine when metadata needs to
   be updated. As a precondtition we had to change decomposedfs as well: to consistently
   calculate the etag for the file content we now store the mtime in the metadata and use the
   metadata lock for atomicity.

   https://github.com/cs3org/reva/pull/4117

*   Bugfix #4081: Propagate sizeDiff

   When postprocessing failed the sizeDiff would not be propagated correctly. This is fixed

   https://github.com/cs3org/reva/pull/4081

*   Bugfix #4091: Register WebDAV HTTP methods with chi

   We now correctly register the WebDAV methods with chi during init.

   https://github.com/owncloud/ocis/issues/6924
   https://github.com/cs3org/reva/pull/4091

*   Bugfix #4107: Return lock when requested

   We did not explictly return the lock when it was requested. This lead to the lock only being
   included when no other metadata was requested. We fixed it by explictly returning the lock when
   requested.

   https://github.com/cs3org/reva/pull/4107

*   Bugfix #4075: Revert 4065 - bypass proxy on upload

   We have reverted PR #4065 to bypass proxy on upload, because it caused issues with oCis.

   https://github.com/cs3org/reva/pull/4075
   https://github.com/cs3org/reva/pull/4065

*   Enhancement #4089: Async propagation (experimental)

   Decomposedfs can now be configured to propagate treetime/treesize changes asynchronously.

   https://github.com/cs3org/reva/pull/4089
   https://github.com/cs3org/reva/pull/4070

*   Enhancement #4074: Allow configuring the max size of grpc messages

   We added a possibility to make the max size of grpc messsages configurable. It is only
   configurable via envvar `OCIS_GRPC_MAX_RECEIVED_MESSAGE_SIZE` . It is recommended to use
   this envvar only temporarily.

   https://github.com/cs3org/reva/pull/4074

*   Enhancement #4083: Allow for rolling back migrations

   The decomposedfs now supports rolling back migrations (starting with 0004). It also got a
   Migrations() method which returns the list of migrations incl. their states.

   https://github.com/cs3org/reva/pull/4083

*   Enhancement #4014: En-/Disable DEPTH:inifinity in PROPFIND

   We have added the ability to en-/disable DEPTH:infinitiy in PROPFIND requests for spaces

   https://github.com/owncloud/ocis/issues/4188
   https://github.com/cs3org/reva/pull/4014

*   Enhancement #4072: Allow to specify a shutdown timeout

   When setting `graceful_shutdown_timeout` revad will try to shutdown in a graceful manner
   when receiving an INT or TERM signal (similar to how it already behaves on SIGQUIT). This allows
   ongoing operations to complete before exiting.

   If the shutdown didn't finish before `graceful_shutdown_timeout` seconds the process will
   exit with an error code (1).

   https://github.com/cs3org/reva/pull/4072

*   Enhancement #4103: Add .oform mimetype

   We switched to a local list of mimetypes and added support for the .oform file extension.

   https://github.com/cs3org/reva/pull/4103
   https://github.com/cs3org/reva/pull/4092

*   Enhancement #4098: Allow naming nats connections

   Bump go-micro and use new `Name` option to pass a connection name

   https://github.com/cs3org/reva/pull/4098

*   Enhancement #4085: Add registry refresh

   We have added registry auto-refresh and made it configurable

   https://github.com/owncloud/ocis/issues/6793
   https://github.com/owncloud/ocis/issues/3832
   https://github.com/cs3org/reva/pull/4085
   https://github.com/owncloud/ocis/pull/6910

*   Enhancement #4097: Remove app ticker logs

   https://github.com/cs3org/reva/pull/4097
   Logs
   would
   show
   regardless
   of
   log
   level
   as
   there
   is
   no
   configuration
   done
   beforehand.
   We
   remove
   the
   logs
   for
   now.

*   Enhancement #4090: Add Capability for sse

   Add a capability for server sent events

   https://github.com/cs3org/reva/pull/4090

*   Enhancement #4110: Tracing events propgation

   Tracing information will now be propagated via events

   https://github.com/cs3org/reva/pull/4110

