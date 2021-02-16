
---
title: "v1.6.0"
linkTitle: "v1.6.0"
weight: 40
description: >
  Changelog for Reva v1.6.0 (2021-02-16)
---

Changelog for reva 1.6.0 (2021-02-16)
=======================================

The following sections list the changes in reva 1.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1425: Align href URL encoding with oc10
 * Fix #1461: Fix public link webdav permissions
 * Fix #1457: Cloning of internal mesh data lost some values
 * Fix #1429: Purge non-empty dirs from trash-bin
 * Fix #1408: Get error status from trash-bin response
 * Enh #1451: Render additional share with in ocs sharing api
 * Enh #1424: We categorized the list of expected failures
 * Enh #1434: CERNBox REST driver for groupprovider service
 * Enh #1400: Checksum support
 * Enh #1431: Update npm packages to fix vulnerabilities
 * Enh #1415: Indicate in EOS containers that TUS is not supported
 * Enh #1402: Parse EOS sys ACLs to generate CS3 resource permissions
 * Enh #1477: Set quota when creating home directory in EOS
 * Enh #1416: Use updated etag of home directory even if it is cached
 * Enh #1478: Enhance error handling for grappa REST drivers
 * Enh #1453: Add functionality to share resources with groups
 * Enh #99: Add stubs and manager for groupprovider service
 * Enh #1462: Hash public share passwords
 * Enh #1464: LDAP driver for the groupprovider service
 * Enh #1430: Capture non-deterministic behavior on storages
 * Enh #1456: Fetch user groups in OIDC and LDAP backend
 * Enh #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore
 * Enh #1467: Align default location for xrdcopy binary

Details
-------

 * Bugfix #1425: Align href URL encoding with oc10

   We now use the same percent encoding for URLs in WebDAV href properties as ownCloud 10.

   https://github.com/owncloud/ocis/issues/1120
   https://github.com/owncloud/ocis/issues/1296
   https://github.com/owncloud/ocis/issues/1307
   https://github.com/cs3org/reva/pull/1425
   https://github.com/cs3org/reva/pull/1472

 * Bugfix #1461: Fix public link webdav permissions

   We now correctly render `oc:permissions` on the root collection of a publicly shared folder
   when it has more than read permissions.

   https://github.com/cs3org/reva/pull/1461

 * Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

 * Bugfix #1429: Purge non-empty dirs from trash-bin

   This wasn't possible before if the directory was not empty

   https://github.com/cs3org/reva/pull/1429

 * Bugfix #1408: Get error status from trash-bin response

   Previously the status code was gathered from the wrong response.

   https://github.com/cs3org/reva/pull/1408

 * Enhancement #1451: Render additional share with in ocs sharing api

   Recipients can now be distinguished by their email, which is rendered as additional info in the
   ocs api for share and file owners as well as share recipients.

   https://github.com/owncloud/ocis/issues/1190
   https://github.com/cs3org/reva/pull/1451

 * Enhancement #1424: We categorized the list of expected failures

   We categorized all expected failures into _File_ (Basic file management like up and download,
   move, copy, properties, trash, versions and chunking), _Sync_ (Synchronization features
   like etag propagation, setting mtime and locking files), _Share_ (File and sync features in a
   shared scenario), _User management_ (User and group management features) and _Other_ (API,
   search, favorites, config, capabilities, not existing endpoints, CORS and others). The
   [Review and fix the tests that have sharing step to work with
   ocis](https://github.com/owncloud/core/issues/38006) reference has been removed, as we
   now have the sharing category

   https://github.com/owncloud/core/issues/38006
   https://github.com/cs3org/reva/pull/1424

 * Enhancement #1434: CERNBox REST driver for groupprovider service

   https://github.com/cs3org/reva/pull/1434

 * Enhancement #1400: Checksum support

   We now support checksums on file uploads and PROPFIND results. On uploads, the ocdav service
   now forwards the `OC-Checksum` (and the similar TUS `Upload-Checksum`) header to the storage
   provider. We added an internal http status code that allows storage drivers to return checksum
   errors. On PROPFINDs, ocdav now renders the `<oc:checksum>` header in a bug compatible way for
   oc10 backward compatibility with existing clients. Finally, GET and HEAD requests now return
   the `OC-Checksum` header.

   https://github.com/owncloud/ocis/issues/1291
   https://github.com/owncloud/ocis/issues/1316
   https://github.com/cs3org/reva/pull/1400
   https://github.com/owncloud/core/pull/38304

 * Enhancement #1431: Update npm packages to fix vulnerabilities

   https://github.com/cs3org/reva/pull/1431

 * Enhancement #1415: Indicate in EOS containers that TUS is not supported

   The OCDAV propfind response previously hardcoded the TUS headers due to which clients such as
   phoenix used the TUS protocol for uploads, which EOS doesn't support. Now we pass this property
   as an opaque entry in the containers metadata.

   https://github.com/cs3org/reva/pull/1415

 * Enhancement #1402: Parse EOS sys ACLs to generate CS3 resource permissions

   https://github.com/cs3org/reva/pull/1402

 * Enhancement #1477: Set quota when creating home directory in EOS

   https://github.com/cs3org/reva/pull/1477

 * Enhancement #1416: Use updated etag of home directory even if it is cached

   We cache the home directory and shares folder etags as calculating these is an expensive
   process. But if these directories were updated after the previously calculated etag was
   cached, we can ignore this calculation and directly return the new one.

   https://github.com/cs3org/reva/pull/1416

 * Enhancement #1478: Enhance error handling for grappa REST drivers

   https://github.com/cs3org/reva/pull/1478

 * Enhancement #1453: Add functionality to share resources with groups

   https://github.com/cs3org/reva/pull/1453

 * Enhancement #99: Add stubs and manager for groupprovider service

   Recently, there was a separation of concerns with regard to users and groups in CS3APIs. This PR
   adds the required stubs and drivers for the group manager.

   https://github.com/cs3org/cs3apis/pull/99
   https://github.com/cs3org/cs3apis/pull/102
   https://github.com/cs3org/reva/pull/1358

 * Enhancement #1462: Hash public share passwords

   The share passwords were only base64 encoded. Added hashing using bcrypt with configurable
   hash cost.

   https://github.com/cs3org/reva/pull/1462

 * Enhancement #1464: LDAP driver for the groupprovider service

   https://github.com/cs3org/reva/pull/1464

 * Enhancement #1430: Capture non-deterministic behavior on storages

   As a developer creating/maintaining a storage driver I want to be able to validate the
   atomicity of all my storage driver operations. * Test for: Start 2 uploads, pause the first one,
   let the second one finish first, resume the first one at some point in time. Both uploads should
   finish. Needs to result in 2 versions, last finished is the most recent version. * Test for:
   Start 2 MKCOL requests with the same path, one needs to fail.

   https://github.com/cs3org/reva/pull/1430

 * Enhancement #1456: Fetch user groups in OIDC and LDAP backend

   https://github.com/cs3org/reva/pull/1456

 * Enhancement #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore

   We added a new storage driver (s3ng) which stores the file metadata on a local filesystem
   (reusing the decomposed filesystem of the ocis driver) and the actual content as blobs in any
   s3-compatible blobstore.

   https://github.com/cs3org/reva/pull/1429

 * Enhancement #1467: Align default location for xrdcopy binary

   https://github.com/cs3org/reva/pull/1467


