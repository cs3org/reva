
---
title: "v2.20.0"
linkTitle: "v2.20.0"
weight: 40
description: >
  Changelog for Reva v2.20.0 (2024-06-19)
---

Changelog for reva 2.20.0 (2024-06-19)
=======================================

The following sections list the changes in reva 2.20.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4623: Consistently use spaceid and nodeid in logs
*   Fix #4584: Prevent copying a file to a parent folder
*   Fix #4700: Clean empty trash node path on delete
*   Fix #4567: Fix error message in authprovider if user is not found
*   Fix #4615: Write blob based on session id
*   Fix #4557: Fix ceph build
*   Fix #4711: Duplicate headers in DAV responses
*   Fix #4568: Fix sharing invite on virtual drive
*   Fix #4559: Fix graph drive invite
*   Fix #4593: Make initiatorIDs also work on uploads
*   Fix #4608: Use gateway selector in jsoncs3
*   Fix #4546: Fix the mount points naming
*   Fix #4678: Fix nats encoding
*   Fix #4630: Fix ocm-share-id
*   Fix #4518: Fix an error when lock/unlock a file
*   Fix #4622: Fix public share update
*   Fix #4566: Fix public link previews
*   Fix #4589: Fix uploading via a public link
*   Fix #4660: Fix creating documents in nested folders of public shares
*   Fix #4635: Fix nil pointer when removing groups from space
*   Fix #4709: Fix share update
*   Fix #4661: Fix space share update for ocs
*   Fix #4656: Fix space share update
*   Fix #4561: Fix Stat() by Path on re-created resource
*   Fix #4710: Tolerate missing user space index
*   Fix #4632: Fix access to files withing a public link targeting a space root
*   Fix #4603: Mask user email in output
*   Chg #4542: Drop unused service spanning stat cache
*   Enh #4712: Add the error translation to the utils
*   Enh #4696: Add List method to ocis and s3ng blobstore
*   Enh #4693: Add mimetype for sb3 files
*   Enh #4699: Add a Path method to blobstore
*   Enh #4695: Add photo and image props
*   Enh #4706: Add secureview flag when listing apps via http
*   Enh #4585: Move more consistency checks to the usershare API
*   Enh #4702: Added theme capability
*   Enh #4672: Add virus filter to list uploads sessions
*   Enh #4614: Bump mockery to v2.40.2
*   Enh #4621: Use a memory cache for the personal space creation cache
*   Enh #4556: Allow tracing requests by giving util functions a context
*   Enh #4694: Expose SecureView in WebDAV permissions
*   Enh #4652: Better error codes when removing a space member
*   Enh #4725: Unique share mountpoint name
*   Enh #4689: Extend service account permissions
*   Enh #4545: Extend service account permissions
*   Enh #4581: Make decomposedfs more extensible
*   Enh #4564: Send file locked/unlocked events
*   Enh #4730: Improve posixfs storage driver
*   Enh #4587: Allow passing a initiator id
*   Enh #4645: Add ItemID to LinkRemoved
*   Enh #4686: Mint view only token for open in app requests
*   Enh #4606: Remove resharing
*   Enh #4643: Secure viewer share role
*   Enh #4631: Add space-share-updated event
*   Enh #4685: Support t and x in ACEs
*   Enh #4625: Test async processing cornercases
*   Enh #4653: Allow to resolve public shares without the ocs tokeninfo endpoint
*   Enh #4657: Add ScanData to Uploadsession

Details
-------

*   Bugfix #4623: Consistently use spaceid and nodeid in logs

   Sometimes we tried to log a node which led to a JSON recursion error because it contains a
   reference to the space root, which references itself. We now always log `spaceid` and
   `nodeid`.

   https://github.com/cs3org/reva/pull/4623

*   Bugfix #4584: Prevent copying a file to a parent folder

   When copying a file to its parent folder, the file would be copied onto the parent folder, moving
   the original folder to the trash-bin.

   https://github.com/owncloud/ocis/issues/1230
   https://github.com/cs3org/reva/pull/4584
   https://github.com/cs3org/reva/pull/4582
   https://github.com/cs3org/reva/pull/4571

*   Bugfix #4700: Clean empty trash node path on delete

   We now delete empty directories in the trash when an item is purged or restored. This prevents
   old empty directories from slowing down the globbing of trash items.

   https://github.com/cs3org/reva/pull/4700

*   Bugfix #4567: Fix error message in authprovider if user is not found

   https://github.com/cs3org/reva/pull/4567

*   Bugfix #4615: Write blob based on session id

   Decomposedfs now uses the session id and size when moving an uplode to the blobstore. This fixes
   a cornercase that prevents an upload session from correctly being finished when another
   upload session to the file was started and already finished.

   https://github.com/cs3org/reva/pull/4615

*   Bugfix #4557: Fix ceph build

   https://github.com/cs3org/reva/pull/4557

*   Bugfix #4711: Duplicate headers in DAV responses

   We fixed an issue where the DAV response headers were duplicated. This was caused by the WebDav
   handler which copied over all headers from the datagateways response. Now, only the relevant
   headers are copied over to the DAV response to prevent duplication.

   https://github.com/cs3org/reva/pull/4711

*   Bugfix #4568: Fix sharing invite on virtual drive

   We fixed the issue when sharing of virtual drive with other users was allowed

   https://github.com/owncloud/ocis/issues/8495
   https://github.com/cs3org/reva/pull/4568

*   Bugfix #4559: Fix graph drive invite

   We fixed the issue when sharing of personal drive is allowed via graph

   https://github.com/owncloud/ocis/issues/8494
   https://github.com/cs3org/reva/pull/4559

*   Bugfix #4593: Make initiatorIDs also work on uploads

   One needs to pass them on initateupload already.

   https://github.com/cs3org/reva/pull/4593

*   Bugfix #4608: Use gateway selector in jsoncs3

   The jsoncs3 user share manager now uses the gateway selector to get a fresh client before making
   requests and uses the configured logger from the context.

   https://github.com/cs3org/reva/pull/4608

*   Bugfix #4546: Fix the mount points naming

   We fixed a bug that caused inconsistent naming when multiple users share the resource with same
   name to another user.

   https://github.com/owncloud/ocis/issues/8471
   https://github.com/cs3org/reva/pull/4546

*   Bugfix #4678: Fix nats encoding

   Encode nats-js-kv keys. This got lost by a dependency bump.

   https://github.com/cs3org/reva/pull/4678

*   Bugfix #4630: Fix ocm-share-id

   We now use the share id to correctly identify ocm shares.

   https://github.com/cs3org/reva/pull/4630

*   Bugfix #4518: Fix an error when lock/unlock a file

   We fixed a bug when anonymous user with viewer role in public link of a folder can lock/unlock a
   file inside it

   https://github.com/owncloud/ocis/issues/7785
   https://github.com/cs3org/reva/pull/4518

*   Bugfix #4622: Fix public share update

   We fixed the permission check for updating public shares. When updating the permissions of a
   public share while not providing a password, the check must be against the new permissions to
   take into account that users can opt out only for view permissions.

   https://github.com/cs3org/reva/pull/4622

*   Bugfix #4566: Fix public link previews

   Fixes previews for public links

   https://github.com/cs3org/reva/pull/4566

*   Bugfix #4589: Fix uploading via a public link

   Fix http error when uploading via a public link

   https://github.com/owncloud/ocis/issues/8699
   https://github.com/cs3org/reva/pull/4589

*   Bugfix #4660: Fix creating documents in nested folders of public shares

   We fixed a bug that prevented creating new documented in a nested folder of a public share.

   https://github.com/owncloud/ocis/issues/8957
   https://github.com/cs3org/reva/pull/4660

*   Bugfix #4635: Fix nil pointer when removing groups from space

   We fixed the nil pointer when removing groups from space via graph

   https://github.com/owncloud/ocis/issues/8768
   https://github.com/cs3org/reva/pull/4635

*   Bugfix #4709: Fix share update

   We fixed the response code when the role/permission is empty on the share update

   https://github.com/owncloud/ocis/issues/8747
   https://github.com/cs3org/reva/pull/4709

*   Bugfix #4661: Fix space share update for ocs

   We fixed the space share update for ocs.

   https://github.com/owncloud/ocis/issues/8905
   https://github.com/cs3org/reva/pull/4661

*   Bugfix #4656: Fix space share update

   We fixed the permission check for updating the space shares when update an expirationDateTime
   only.

   https://github.com/owncloud/ocis/issues/8905
   https://github.com/cs3org/reva/pull/4656

*   Bugfix #4561: Fix Stat() by Path on re-created resource

   We fixed bug that caused Stat Requests using a Path reference to a mount point in the sharejail to
   not resolve correctly, when a share using the same mount point to an already deleted resource
   was still existing.

   https://github.com/owncloud/ocis/issues/7895
   https://github.com/cs3org/reva/pull/4561

*   Bugfix #4710: Tolerate missing user space index

   We fixed a bug where the spaces for a user were not listed if the user had no space index by user.
   This happens when a user has the role "User Light" and has been invited to a project space via a
   group.

   https://github.com/cs3org/reva/pull/4710

*   Bugfix #4632: Fix access to files withing a public link targeting a space root

   We fixed an issue that prevented users from opening documents within a public share that
   targets a space root.

   https://github.com/owncloud/ocis/issues/8691
   https://github.com/cs3org/reva/pull/4632/

*   Bugfix #4603: Mask user email in output

   We have fixed a bug where the user email was not masked in the output and the user emails could be
   enumerated through the sharee search.

   https://github.com/owncloud/ocis/issues/8726
   https://github.com/cs3org/reva/pull/4603

*   Change #4542: Drop unused service spanning stat cache

   We removed the stat cache shared between gateway and storage providers. It is constantly
   invalidated and needs a different approach.

   https://github.com/cs3org/reva/pull/4542

*   Enhancement #4712: Add the error translation to the utils

   We've added the error translation from the statusCodeError type to CS3 Status

   https://github.com/owncloud/ocis/issues/9151
   https://github.com/cs3org/reva/pull/4712

*   Enhancement #4696: Add List method to ocis and s3ng blobstore

   Allow listing blobstores for maintenance

   https://github.com/cs3org/reva/pull/4696

*   Enhancement #4693: Add mimetype for sb3 files

   We've added the matching mimetype for sb3 files

   https://github.com/cs3org/reva/pull/4693

*   Enhancement #4699: Add a Path method to blobstore

   Add a method to get the path of a blob to the ocis and s3ng blobstores.

   https://github.com/cs3org/reva/pull/4699

*   Enhancement #4695: Add photo and image props

   Add `oc:photo` and `oc:image` props to PROPFIND responses for propall requests or when they
   are explicitly requested.

   https://github.com/cs3org/reva/pull/4695
   https://github.com/cs3org/reva/pull/4684

*   Enhancement #4706: Add secureview flag when listing apps via http

   To allow clients to see which application supports secure view we add a flag to the http response
   when the app address matches a configured secure view app address.

   https://github.com/cs3org/reva/pull/4706
   https://github.com/cs3org/reva/pull/4703

*   Enhancement #4585: Move more consistency checks to the usershare API

   The gateway now checks if there will be at least one space manager remaining before deleting a
   space member. The legacy ocs based sharing implementaion already does this on its own. But for
   the future graph based sharing implementation it is better to have the check in a more central
   place.

   https://github.com/cs3org/reva/pull/4585

*   Enhancement #4702: Added theme capability

   The ocs capabilities now contain a theme capability that exposes theme related configuration
   options to the clients. As part of this change, the ocs capabilities are now exposed and can be
   used externally.

   https://github.com/cs3org/reva/pull/4702

*   Enhancement #4672: Add virus filter to list uploads sessions

   Adds a filter for filtering for infected uploads

   https://github.com/cs3org/reva/pull/4672

*   Enhancement #4614: Bump mockery to v2.40.2

   We switched to the latest mockery and changed to .mockery.yaml based mock generation.

   https://github.com/cs3org/reva/pull/4614

*   Enhancement #4621: Use a memory cache for the personal space creation cache

   Memory is a safe default and ensures that superfluous calls to CreateStorageSpace are
   prevented.

   https://github.com/cs3org/reva/pull/4621

*   Enhancement #4556: Allow tracing requests by giving util functions a context

   We deprecated GetServiceUserContext with GetServiceUserContextWithContext and GetUser
   with GetUserWithContext to allow passing in a trace context.

   https://github.com/cs3org/reva/pull/4556

*   Enhancement #4694: Expose SecureView in WebDAV permissions

   When a file or folder can be securely viewed we now add an `X` to the permissions.

   https://github.com/cs3org/reva/pull/4694

*   Enhancement #4652: Better error codes when removing a space member

   The gateway returns more specific error codes when removing a space member fails.

   https://github.com/owncloud/ocis/issues/8819
   https://github.com/cs3org/reva/pull/4652

*   Enhancement #4725: Unique share mountpoint name

   Accepting a received share with a mountpoint name that already exists will now append a unique
   suffix to the mountpoint name.

   https://github.com/owncloud/ocis/issues/8961
   https://github.com/cs3org/reva/pull/4725
   https://github.com/cs3org/reva/pull/4723
   https://github.com/cs3org/reva/pull/4714

*   Enhancement #4689: Extend service account permissions

   Adds AddGrant permisson

   https://github.com/cs3org/reva/pull/4689

*   Enhancement #4545: Extend service account permissions

   Adds CreateContainer permisson and improves cs3 storage pkg

   https://github.com/cs3org/reva/pull/4545

*   Enhancement #4581: Make decomposedfs more extensible

   We refactored decomposedfs to make it more extensible, e.g. for the posixfs storage driver.

   https://github.com/cs3org/reva/pull/4581

*   Enhancement #4564: Send file locked/unlocked events

   Emit an event when a file is locked or unlocked

   https://github.com/cs3org/reva/pull/4564

*   Enhancement #4730: Improve posixfs storage driver

   Improve the posixfs storage driver by fixing several issues and adding missing features.

   https://github.com/cs3org/reva/pull/4730
   https://github.com/cs3org/reva/pull/4719
   https://github.com/cs3org/reva/pull/4708
   https://github.com/cs3org/reva/pull/4562

*   Enhancement #4587: Allow passing a initiator id

   Allows passing an initiator id on http request as `Initiator-ID` header. It will be passed down
   though ocis and returned with sse events (clientlog events, as userlog has its own logic)

   https://github.com/cs3org/reva/pull/4587

*   Enhancement #4645: Add ItemID to LinkRemoved

   Add itemID to linkremoved response and event

   https://github.com/cs3org/reva/pull/4645

*   Enhancement #4686: Mint view only token for open in app requests

   When a view only mode is requested for open in app requests the gateway now mints a view only token
   scoped to the requested resource. This token can be used by trusted app providers to download
   the resource even if the user has no download permission.

   https://github.com/cs3org/reva/pull/4686

*   Enhancement #4606: Remove resharing

   Removed all code related to resharing

   https://github.com/cs3org/reva/pull/4606

*   Enhancement #4643: Secure viewer share role

   A new share role "Secure viewer" has been added. This role only allows viewing resources, no
   downloading, editing or deleting.

   https://github.com/cs3org/reva/pull/4643

*   Enhancement #4631: Add space-share-updated event

   This event is triggered when a share on a space root (aka space membership) is changed

   https://github.com/cs3org/reva/pull/4631

*   Enhancement #4685: Support t and x in ACEs

   To support view only shares (dowload forbidden) we added t (read attrs) and x (directory
   traversal) permissions to the decomposed FS ACEs.

   https://github.com/cs3org/reva/pull/4685

*   Enhancement #4625: Test async processing cornercases

   We added tests to cover several bugs where file metadata or parent treesize might get corrupted
   when postprocessing errors occur in specific order. For now, the added test cases test the
   current behavior but contain comments and FIXMEs for the expected behavior.

   https://github.com/cs3org/reva/pull/4625

*   Enhancement #4653: Allow to resolve public shares without the ocs tokeninfo endpoint

   Instead of querying the /v1.php/apps/files_sharing/api/v1/tokeninfo/ endpoint, a client
   can now resolve public and internal links by sending a PROPFIND request to
   /dav/public-files/{sharetoken}

   * authenticated clients accessing an internal link are redirected to the "real" resource
   (`/dav/spaces/{target-resource-id} * authenticated clients are able to resolve public
   links like before. For password protected links they need to supply the password even if they
   have access to the underlying resource by other means. * unauthenticated clients accessing an
   internal link get a 401 returned with WWW-Authenticate set to Bearer (so that the client knows
   that it need to get a token via the IDP login page. * unauthenticated clients accessing a
   password protected link get a 401 returned with an error message to indicate the requirement
   for needing the link's password.

   https://github.com/owncloud/ocis/issues/8858
   https://github.com/cs3org/reva/pull/4653

*   Enhancement #4657: Add ScanData to Uploadsession

   Adds virus scan results to the upload session.

   https://github.com/cs3org/reva/pull/4657

