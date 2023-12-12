
---
title: "v2.17.0"
linkTitle: "v2.17.0"
weight: 40
description: >
  Changelog for Reva v2.17.0 (2023-12-12)
---

Changelog for reva 2.17.0 (2023-12-12)
=======================================

The following sections list the changes in reva 2.17.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4278: Disable DEPTH infinity in PROPFIND
*   Fix #4318: Do not allow moves between shares
*   Fix #4290: Prevent panic when trying to move a non-existent file
*   Fix #4241: Allow an empty credentials chain in the auth middleware
*   Fix #4216: Fix an error message
*   Fix #4324: Fix capabilities decoding
*   Fix #4267: Fix concurrency issue
*   Fix #4362: Fix concurrent lookup
*   Fix #4336: Fix definition of "file-editor" role
*   Fix #4302: Fix checking of filename length
*   Fix #4366: Fix CS3 status code when looking up non existing share
*   Fix #4299: Fix HTTP verb of the generate-invite endpoint
*   Fix #4249: GetUserByClaim not working with MSAD for claim "userid"
*   Fix #4217: Fix missing case for "hide" in UpdateShares
*   Fix #4140: Fix missing etag in shares jail
*   Fix #4229: Fix destroying the Personal and Project spaces data
*   Fix #4193: Fix overwrite a file with an empty file
*   Fix #4365: Fix create public share
*   Fix #4380: Fix the public link update
*   Fix #4250: Fix race condition
*   Fix #4345: Fix conversion of custom ocs permissions to roles
*   Fix #4134: Fix share jail
*   Fix #4335: Fix public shares cleanup config
*   Fix #4338: Fix unlock via space API
*   Fix #4341: Fix spaceID in meta endpoint response
*   Fix #4351: Fix 500 when open public link
*   Fix #4352: Fix the tgz mime type
*   Fix #4388: Allow UpdateUserShare() to update just the expiration date
*   Fix #4214: Always pass adjusted default nats options
*   Fix #4291: Release lock when expired
*   Fix #4386: Remove dead enable_home config
*   Fix #4292: Return 403 when user is not permitted to log
*   Enh #4389: Add audio and location props
*   Enh #4337: Check permissions before creating shares
*   Enh #4326: Add search mediatype filter
*   Enh #4367: Add GGS mime type
*   Enh #4295: Add hide flag to shares
*   Enh #4358: Add default permissions capability for links
*   Enh #4133: Add more metadata to locks
*   Enh #4353: Add support for .docxf files
*   Enh #4363: Add nats-js-kv store
*   Enh #4197: Add the Banned-Passwords List
*   Enh #4190: Add the password policies
*   Enh #4384: Add a retry postprocessing outcome and event
*   Enh #4271: Add search capability
*   Enh #4119: Add sse event
*   Enh #4392: Add additional permissions to service accounts
*   Enh #4344: Add url extension to mime type list
*   Enh #4372: Add validation to the public share provider
*   Enh #4244: Allow listing reveived shares by service accounts
*   Enh #4129: Auto-Accept Shares through ServiceAccounts
*   Enh #4374: Handle trashbin file listings concurrently
*   Enh #4325: Enforce Permissions
*   Enh #4368: Extract log initialization
*   Enh #4375: Introduce UploadSessionLister interface
*   Enh #4268: Implement sharing roles
*   Enh #4160: Improve utils pkg
*   Enh #4335: Add sufficient permissions check function
*   Enh #4281: Port OCM changes from master
*   Enh #4270: Opt out of public link password enforcement
*   Enh #4181: The password policies change request
*   Enh #4256: Rename hidden share variable name
*   Enh #3315: Accept reva token as a bearer authentication
*   Enh #3926: Service Accounts
*   Enh #4359: Update go-ldap to v3.4.6
*   Enh #4170: Update password policies
*   Enh #4232: Improve error handling in utils package

Details
-------

*   Bugfix #4278: Disable DEPTH infinity in PROPFIND

   Disabled DEPTH infinity in PROPFIND for Personal /remote.php/dav/files/admin Public link
   share /remote.php/dav/public-files/<token> Trashbin
   /remote.php/dav/spaces/trash-bin/<personal-space-id>

   https://github.com/owncloud/ocis/issues/7359
   https://github.com/cs3org/reva/pull/4278

*   Bugfix #4318: Do not allow moves between shares

   We no longer allow moves between shares, even if they resolve to the same space.

   https://github.com/cs3org/reva/pull/4318

*   Bugfix #4290: Prevent panic when trying to move a non-existent file

   We fixed a panic when the user tried to move a file which does not exist.

   https://github.com/cs3org/reva/pull/4290
   https://github.com/cs3org/reva/pull/4283

*   Bugfix #4241: Allow an empty credentials chain in the auth middleware

   When running with ocis, all external http-authentication is handled by the proxy service. So
   the reva auth middleware should not try to do any basic or bearer auth.

   https://github.com/owncloud/ocis/issues/6692
   https://github.com/cs3org/reva/pull/4241

*   Bugfix #4216: Fix an error message

   Capitalize an error message for Banned-Passwords List OCIS-3809

   https://github.com/cs3org/reva/pull/4216

*   Bugfix #4324: Fix capabilities decoding

   We have fixed a bug when the mapstructure is ignored the embedded structure when decode

   https://github.com/cs3org/reva/pull/4324

*   Bugfix #4267: Fix concurrency issue

   We fixed a concurrency issue when listing received shares.

   https://github.com/cs3org/reva/pull/4267

*   Bugfix #4362: Fix concurrent lookup

   We have fixed a bug that overwrites existing variables, leading to flaky lookup of spaces

   https://github.com/cs3org/reva/pull/4362

*   Bugfix #4336: Fix definition of "file-editor" role

   The "file-editor" role was missing the AddGrant resource permission, which caused a broken
   mapping from ResourcePermissions to roles in certain cases.

   https://github.com/cs3org/reva/pull/4336

*   Bugfix #4302: Fix checking of filename length

   Instead of checking for length of the filename the ocdav handler would sometimes check for
   complete file path.

   https://github.com/cs3org/reva/pull/4302

*   Bugfix #4366: Fix CS3 status code when looking up non existing share

   When trying to lookup a share that does not exist we now return a proper "not found" error instead
   of just an "internal error.

   https://github.com/cs3org/reva/pull/4366

*   Bugfix #4299: Fix HTTP verb of the generate-invite endpoint

   We changed the HTTP verb of the /generate-invite endpoint of the sciencemesh service to POST as
   it clearly has side effects for the system, it's not just a read-only call.

   https://github.com/cs3org/reva/pull/4299

*   Bugfix #4249: GetUserByClaim not working with MSAD for claim "userid"

   We fixed GetUserByClaim to correctly deal with binary encoded userid as e.g. used for Active
   Directory.

   https://github.com/owncloud/ocis/issues/7469
   https://github.com/cs3org/reva/pull/4249

*   Bugfix #4217: Fix missing case for "hide" in UpdateShares

   We fixed a bug that caused ocs to throw a 996 on update of permissions.

   https://github.com/cs3org/reva/pull/4217

*   Bugfix #4140: Fix missing etag in shares jail

   The shares jail can miss the etag if the first `receivedShare` is not accepted.

   https://github.com/cs3org/reva/pull/4140

*   Bugfix #4229: Fix destroying the Personal and Project spaces data

   We fixed a bug that caused destroying the Personal and Project spaces data when providing as a
   destination while move/copy file. Disallow use the Personal and Project spaces root as a
   source while move/copy file.

   https://github.com/owncloud/ocis/issues/6739
   https://github.com/cs3org/reva/pull/4229

*   Bugfix #4193: Fix overwrite a file with an empty file

   Fix the error when the user trying to overwrite a file with an empty file

   https://github.com/cs3org/reva/pull/4193

*   Bugfix #4365: Fix create public share

   If public link creation failed, it now returns a status error instead of sending ok.

   https://github.com/cs3org/reva/pull/4365

*   Bugfix #4380: Fix the public link update

   We fixed a bug when normal users can update the public link to delete its password if permission
   is not sent in data.

   https://github.com/owncloud/ocis/issues/7821
   https://github.com/cs3org/reva/pull/4380

*   Bugfix #4250: Fix race condition

   We have fixed a race condition when setting the default tracing provider.

   https://github.com/owncloud/ocis/issues/4088
   https://github.com/cs3org/reva/pull/4250

*   Bugfix #4345: Fix conversion of custom ocs permissions to roles

   When creating shares with custom permissions they were under certain conditions converted
   into the wrong corrensponding sharing role

   https://github.com/owncloud/enterprise/issues/6209
   https://github.com/cs3org/reva/pull/4345
   https://github.com/cs3org/reva/pull/4343
   https://github.com/cs3org/reva/pull/4342

*   Bugfix #4134: Fix share jail

   Make matching mountpoints deterministic by comparing whole path segments of mountpoints

   https://github.com/cs3org/reva/pull/4134

*   Bugfix #4335: Fix public shares cleanup config

   The public shares cleanup for expired shares was not configurable via ocis.

   https://github.com/cs3org/reva/pull/4335/

*   Bugfix #4338: Fix unlock via space API

   We fixed a bug that caused Error 500 when user try to unlock file using fileid The
   handleSpaceUnlock has been added

   https://github.com/owncloud/ocis/issues/7708
   https://github.com/cs3org/reva/pull/4338

*   Bugfix #4341: Fix spaceID in meta endpoint response

   When doing a `PROPFIND` on the meta endpoint the spaceID would not be rendered correctly. That
   is fixed now

   https://github.com/cs3org/reva/pull/4341

*   Bugfix #4351: Fix 500 when open public link

   We fixed a bug that caused nil pointer and Error 500 when open a public link from a deleted user

   https://github.com/owncloud/ocis/issues/7740
   https://github.com/cs3org/reva/pull/4351

*   Bugfix #4352: Fix the tgz mime type

   We have fixed a bug when the tgz mime type was not "application/gzip"

   https://github.com/cs3org/reva/pull/4352

*   Bugfix #4388: Allow UpdateUserShare() to update just the expiration date

   The UpdateUserShare Request now works if it just contains an update of the expiration date.

   https://github.com/cs3org/reva/pull/4388

*   Bugfix #4214: Always pass adjusted default nats options

   The nats-js store will now automatically reconnect.

   https://github.com/cs3org/reva/pull/4214

*   Bugfix #4291: Release lock when expired

   Release an expired lock when stating the resource

   https://github.com/cs3org/reva/pull/4291

*   Bugfix #4386: Remove dead enable_home config

   https://github.com/cs3org/reva/pull/4386

*   Bugfix #4292: Return 403 when user is not permitted to log

   When a user tries to lock a file, but doesn't have write access, the correct status code is `403`
   not `500` like we did until now

   https://github.com/cs3org/reva/pull/4292

*   Enhancement #4389: Add audio and location props

   Add `oc:audio` and `oc:location` props to PROPFIND responses for propall requests or when
   they are explicitly requested.

   https://github.com/cs3org/reva/pull/4389

*   Enhancement #4337: Check permissions before creating shares

   The user share provider now checks if the user has sufficient permissions to create a share.

   https://github.com/cs3org/reva/pull/4337/

*   Enhancement #4326: Add search mediatype filter

   Add filter MediaType filter shortcuts to search for specific document types. For example, a
   search query MimeType:documents will search for files with the following mimetypes:

   Application/msword
   MimeType:application/vnd.openxmlformats-officedocument.wordprocessingml.document
   MimeType:application/vnd.oasis.opendocument.text MimeType:text/plain
   MimeType:text/markdown MimeType:application/rtf
   MimeType:application/vnd.apple.pages

   https://github.com/owncloud/ocis/issues/7432
   https://github.com/cs3org/reva/pull/4326
   https://github.com/cs3org/reva/pull/4320

*   Enhancement #4367: Add GGS mime type

   We have added a new mime type for GGS files. This is a new file type that is used by geogebra
   application.

   https://github.com/owncloud/ocis/issues/7768
   https://github.com/cs3org/reva/pull/4367
   https://github.com/owncloud/ocis/pull/7804

*   Enhancement #4295: Add hide flag to shares

   We have added the ability to hide shares through the
   ocs/v2.php/apps/files_sharing/api/v1/shares/pending/ endpoint by appending a
   POST-Variable called hide which can be true or false.

   https://github.com/owncloud/ocis/issues/7589
   https://github.com/cs3org/reva/pull/4295
   https://github.com/cs3org/reva/pull/4289
   https://github.com/cs3org/reva/pull/4194

*   Enhancement #4358: Add default permissions capability for links

   A capability for default permissions for links has been added.

   https://github.com/owncloud/web/issues/9919
   https://github.com/cs3org/reva/pull/4358

*   Enhancement #4133: Add more metadata to locks

   Adds the owners name and the time of locking to the lock metadata

   https://github.com/cs3org/reva/pull/4133

*   Enhancement #4353: Add support for .docxf files

   We have added the missing .docxf mime-type to the list of supported mime-types.

   https://github.com/owncloud/ocis/issues/6989
   https://github.com/cs3org/reva/pull/4353

*   Enhancement #4363: Add nats-js-kv store

   Add a store using the nats-js key value storage. Also fixes a panic when locking files.

   https://github.com/cs3org/reva/pull/4363

*   Enhancement #4197: Add the Banned-Passwords List

   Add ability to validation against the Banned-Passwords List OCIS-3809

   https://github.com/cs3org/reva/pull/4197

*   Enhancement #4190: Add the password policies

   Add the password policies OCIS-3767

   https://github.com/cs3org/reva/pull/4190
   https://github.com/cs3org/reva/pull/4147

*   Enhancement #4384: Add a retry postprocessing outcome and event

   We added a retry postprocessing outcome and event. This enhancement provides the ability to
   handle retry scenarios during postprocessing.

   https://github.com/cs3org/reva/pull/4384

*   Enhancement #4271: Add search capability

   We have added the ability to define search specific capabilities.

   https://github.com/cs3org/reva/pull/4271

*   Enhancement #4119: Add sse event

   Adds an event to issue sse notifications

   https://github.com/cs3org/reva/pull/4119

*   Enhancement #4392: Add additional permissions to service accounts

   We added the `RestoreRecycleItem` and `Delete` permissions to service accounts

   https://github.com/owncloud/ocis/issues/7845
   https://github.com/cs3org/reva/pull/4392

*   Enhancement #4344: Add url extension to mime type list

   We have added the url extension to the mime type list

   https://github.com/cs3org/reva/pull/4344

*   Enhancement #4372: Add validation to the public share provider

   We added validation to the public share provider. The idea behind it is that the cs3 clients will
   become much simpler. The provider can do the validation and return different status codes. The
   API clients then just need to convert CS3 status codes to http status codes.

   https://github.com/owncloud/ocis/issues/6993
   https://github.com/cs3org/reva/pull/4372/

*   Enhancement #4244: Allow listing reveived shares by service accounts

   Similar to UpdateReceivedShare we now pass a forUser parameter to list received shares when
   using service accounts

   https://github.com/cs3org/reva/pull/4244

*   Enhancement #4129: Auto-Accept Shares through ServiceAccounts

   Auto accept shares with service accounts

   https://github.com/cs3org/reva/pull/4129

*   Enhancement #4374: Handle trashbin file listings concurrently

   We now use a concurrent walker to list files in the trashbin. This improves performance when
   listing files in the trashbin.

   https://github.com/owncloud/ocis/issues/7844
   https://github.com/cs3org/reva/pull/4374

*   Enhancement #4325: Enforce Permissions

   Enforce the new `Favorites.List` `Favorites.Write` and `Shares.Write` Permissions

   https://github.com/cs3org/reva/pull/4325

*   Enhancement #4368: Extract log initialization

   To prepare reinitializing a logger for uploads we refactored the loginitialization into its
   own package

   https://github.com/cs3org/reva/pull/4368

*   Enhancement #4375: Introduce UploadSessionLister interface

   We introduced a new UploadSessionLister interface that allows better control of upload
   sessions. Upload sessions include the processing state and can be used to filter and purge the
   list of currently ongoing upload sessions.

   https://github.com/cs3org/reva/pull/4375

*   Enhancement #4268: Implement sharing roles

   Implement libre graph sharing roles

   https://github.com/owncloud/ocis/issues/7418
   https://github.com/cs3org/reva/pull/4268

*   Enhancement #4160: Improve utils pkg

   Add more function to utils pkg so they don't need to be copy/pasted everywhere

   https://github.com/cs3org/reva/pull/4160

*   Enhancement #4335: Add sufficient permissions check function

   We added a helper function to check for sufficient CS3 resource permissions.

   https://github.com/owncloud/ocis/issues/6993
   https://github.com/cs3org/reva/pull/4335/

*   Enhancement #4281: Port OCM changes from master

   We pulled in the latest ocm changes from master and are now compatible with the main go-cs3apis
   again.

   https://github.com/cs3org/reva/pull/4281
   https://github.com/cs3org/reva/pull/4239

*   Enhancement #4270: Opt out of public link password enforcement

   Users with special permissions can now delete passwords on read-only public links.

   https://github.com/owncloud/ocis/issues/7538
   https://github.com/cs3org/reva/pull/4270

*   Enhancement #4181: The password policies change request

   The variables renaming OCIS-3767

   https://github.com/cs3org/reva/pull/4181

*   Enhancement #4256: Rename hidden share variable name

   We have renamed the hidden flag on shares from Hide -> Hidden to align to the cs3api

   https://github.com/cs3org/reva/pull/4256
   https://github.com/cs3org/cs3apis/pull/214

*   Enhancement #3315: Accept reva token as a bearer authentication

   https://github.com/cs3org/reva/pull/3315

*   Enhancement #3926: Service Accounts

   Makes reva ready for service accounts by introducing an serviceaccounts auth manager

   https://github.com/cs3org/reva/pull/3926

*   Enhancement #4359: Update go-ldap to v3.4.6

   Updated go-ldap/ldap/v3 to the latest upstream release to include the latest bugfixes and
   enhancements.

   https://github.com/cs3org/reva/pull/4359

*   Enhancement #4170: Update password policies

   The Password policies have been updated. The special characters list became constant.
   OCIS-3767

   https://github.com/cs3org/reva/pull/4170

*   Enhancement #4232: Improve error handling in utils package

   Improves error handling in the utils package. This has no impact on users.

   https://github.com/cs3org/reva/pull/4232

