
---
title: "v1.7.0"
linkTitle: "v1.7.0"
weight: 40
description: >
  Changelog for Reva v1.7.0 (2021-04-19)
---

Changelog for reva 1.7.0 (2021-04-19)
=======================================

The following sections list the changes in reva 1.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1619: Fixes for enabling file sharing in EOS
 * Fix #1576: Fix etag changing only once a second
 * Fix #1634: Mentix site authorization status changes
 * Fix #1625: Make local file connector more error tolerant
 * Fix #1526: Fix webdav file versions endpoint bugs
 * Fix #1457: Cloning of internal mesh data lost some values
 * Fix #1597: Check for ENOTDIR on readlink error
 * Fix #1636: Skip file check for OCM data transfers
 * Fix #1552: Fix a bunch of trashbin related issues
 * Fix #1: Bump meshdirectory-web to 1.0.2
 * Chg #1562: Modularize api token management in GRAPPA drivers
 * Chg #1452: Separate blobs from metadata in the ocis storage driver
 * Enh #1514: Add grpc test suite for the storage provider
 * Enh #1466: Add integration tests for the s3ng driver
 * Enh #1521: Clarify expected failures
 * Enh #1624: Add wrappers for EOS and EOS Home storage drivers
 * Enh #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType
 * Enh #1411: Make InsecureSkipVerify configurable
 * Enh #1106: Make command to run litmus tests
 * Enh #1502: Bump meshdirectory-web to v1.0.4
 * Enh #1502: New MeshDirectory HTTP service UI frontend with project branding
 * Enh #1405: Quota querying and tree accounting
 * Enh #1527: Add FindAcceptedUsers method to OCM Invite API
 * Enh #1149: Add CLI Commands for OCM invitation workflow
 * Enh #1629: Implement checksums in the owncloud storage
 * Enh #1528: Port drone pipeline definition to starlark
 * Enh #110: Add signature authentication for public links
 * Enh #1495: SQL driver for the publicshare service
 * Enh #1588: Make the additional info attribute for shares configurable
 * Enh #1595: Add site account registration panel
 * Enh #1506: Site Accounts service for API keys
 * Enh #116: Enhance storage registry with virtual views and regular expressions
 * Enh #1513: Add stubs for storage spaces manipulation

Details
-------

 * Bugfix #1619: Fixes for enabling file sharing in EOS

   https://github.com/cs3org/reva/pull/1619

 * Bugfix #1576: Fix etag changing only once a second

   We fixed a problem with the owncloud storage driver only considering the mtime with a second
   resolution for the etag calculation.

   https://github.com/cs3org/reva/pull/1576

 * Bugfix #1634: Mentix site authorization status changes

   If a site changes its authorization status, Mentix did not update its internal data to reflect
   this change. This PR fixes this issue.

   https://github.com/cs3org/reva/pull/1634

 * Bugfix #1625: Make local file connector more error tolerant

   The local file connector caused Reva to throw an exception if the local file for storing site
   data couldn't be loaded. This PR changes this behavior so that only a warning is logged.

   https://github.com/cs3org/reva/pull/1625

 * Bugfix #1526: Fix webdav file versions endpoint bugs

   Etag and error code related bugs have been fixed for the webdav file versions endpoint and
   removed from the expected failures file.

   https://github.com/cs3org/reva/pull/1526

 * Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

 * Bugfix #1597: Check for ENOTDIR on readlink error

   The deconstructed storage driver now handles ENOTDIR errors when `node.Child()` is called
   for a path containing a path segment that is actually a file.

   https://github.com/owncloud/ocis/issues/1239
   https://github.com/cs3org/reva/pull/1597

 * Bugfix #1636: Skip file check for OCM data transfers

   https://github.com/cs3org/reva/pull/1636

 * Bugfix #1552: Fix a bunch of trashbin related issues

   Fixed these issues:

   - Complete: Deletion time in trash bin shows a wrong date - Complete: shared trash status code -
   Partly: invalid webdav responses for unauthorized requests. - Partly: href in trashbin
   PROPFIND response is wrong

   Complete means there are no expected failures left. Partly means there are some scenarios
   left.

   https://github.com/cs3org/reva/pull/1552

 * Bugfix #1: Bump meshdirectory-web to 1.0.2

   Updated meshdirectory-web mod to version 1.0.2 that contains fixes for OCM invite API links
   generation.

   https://github.com/sciencemesh/meshdirectory-web/pull/1

 * Change #1562: Modularize api token management in GRAPPA drivers

   This PR moves the duplicated api token management methods into a seperate utils package

   https://github.com/cs3org/reva/issues/1562

 * Change #1452: Separate blobs from metadata in the ocis storage driver

   We changed the ocis storage driver to keep the file content separate from the metadata by
   storing the blobs in a separate directory. This allows for using a different (potentially
   faster) storage for the metadata.

   **Note** This change makes existing ocis storages incompatible with the new code.

   We also streamlined the ocis and the s3ng drivers so that most of the code is shared between them.

   https://github.com/cs3org/reva/pull/1452

 * Enhancement #1514: Add grpc test suite for the storage provider

   A new test suite has been added which tests the grpc interface to the storage provider. It
   currently runs against the ocis and the owncloud storage drivers.

   https://github.com/cs3org/reva/pull/1514

 * Enhancement #1466: Add integration tests for the s3ng driver

   We extended the integration test suite to also run all tests against the s3ng driver.

   https://github.com/cs3org/reva/pull/1466

 * Enhancement #1521: Clarify expected failures

   Some features, while covered by the ownCloud 10 acceptance tests, will not be implmented for
   now: - blacklisted / ignored files, because ocis/reva don't need to blacklist `.htaccess`
   files - `OC-LazyOps` support was [removed from the
   clients](https://github.com/owncloud/client/pull/8398). We are thinking about [a state
   machine for uploads to properly solve that scenario and also list the state of files in progress
   in the web ui](https://github.com/owncloud/ocis/issues/214). The expected failures
   files now have a dedicated _Won't fix_ section for these items.

   https://github.com/owncloud/ocis/issues/214
   https://github.com/cs3org/reva/pull/1521
   https://github.com/owncloud/client/pull/8398

 * Enhancement #1624: Add wrappers for EOS and EOS Home storage drivers

   For CERNBox, we need the mount ID to be configured according to the owner of a resource. Setting
   this in the storageprovider means having different instances of this service to cater to
   different users, which does not scale. This driver forms a wrapper around the EOS driver and
   sets the mount ID according to a configurable mapping based on the owner of the resource.

   https://github.com/cs3org/reva/pull/1624

 * Enhancement #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType

   Interface method Share() in pkg/ocm/share/share.go now has a share type parameter.

   https://github.com/cs3org/reva/pull/1563

 * Enhancement #1411: Make InsecureSkipVerify configurable

   Add `InsecureSkipVerify` field to `metrics.Config` struct and update examples to include
   it.

   https://github.com/cs3org/reva/issues/1411

 * Enhancement #1106: Make command to run litmus tests

   This updates adds an extra make command to run litmus tests via make. `make litmus-test`
   executes the tests.

   https://github.com/cs3org/reva/issues/1106
   https://github.com/cs3org/reva/pull/1543

 * Enhancement #1502: Bump meshdirectory-web to v1.0.4

   Updated meshdirectory-web version to v.1.0.4 bringing multiple UX improvements in provider
   list and map.

   https://github.com/cs3org/reva/issues/1502
   https://github.com/sciencemesh/meshdirectory-web/pull/2
   https://github.com/sciencemesh/meshdirectory-web/pull/3

 * Enhancement #1502: New MeshDirectory HTTP service UI frontend with project branding

   We replaced the temporary version of web frontend of the mesh directory http service with a new
   redesigned & branded one. Because the new version is a more complex Vue SPA that contains image,
   css and other assets, it is now served from a binary package distribution that was generated
   using the [github.com/rakyll/statik](https://github.com/rakyll/statik) package. The
   `http.services.meshdirectory.static` config option was obsoleted by this change.

   https://github.com/cs3org/reva/issues/1502

 * Enhancement #1405: Quota querying and tree accounting

   The ocs api now returns the user quota for the users home storage. Furthermore, the ocis storage
   driver now reads the quota from the extended attributes of the user home or root node and
   implements tree size accounting. Finally, ocdav PROPFINDS now handle the
   `DAV:quota-used-bytes` and `DAV:quote-available-bytes` properties.

   https://github.com/cs3org/reva/pull/1405
   https://github.com/cs3org/reva/pull/1491

 * Enhancement #1527: Add FindAcceptedUsers method to OCM Invite API

   https://github.com/cs3org/reva/pull/1527

 * Enhancement #1149: Add CLI Commands for OCM invitation workflow

   This adds a couple of CLI commands, `ocm-invite-generate` and `ocm-invite-forward` to
   generate and forward ocm invitation tokens respectively.

   https://github.com/cs3org/reva/issues/1149

 * Enhancement #1629: Implement checksums in the owncloud storage

   Implemented checksums in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1629

 * Enhancement #1528: Port drone pipeline definition to starlark

   Having the pipeline definition as a starlark script instead of plain yaml greatly improves the
   flexibility and allows for removing lots of duplicated definitions.

   https://github.com/cs3org/reva/pull/1528

 * Enhancement #110: Add signature authentication for public links

   Implemented signature authentication for public links in addition to the existing password
   authentication. This allows web clients to efficiently download files from password
   protected public shares.

   https://github.com/cs3org/cs3apis/issues/110
   https://github.com/cs3org/reva/pull/1590

 * Enhancement #1495: SQL driver for the publicshare service

   https://github.com/cs3org/reva/pull/1495

 * Enhancement #1588: Make the additional info attribute for shares configurable

   AdditionalInfoAttribute can be configured via the `additional_info_attribute` key in the
   form of a Go template string. If not explicitly set, the default value is `{{.Mail}}`

   https://github.com/cs3org/reva/pull/1588

 * Enhancement #1595: Add site account registration panel

   This PR adds a site account registration panel to the site accounts service. It also removes
   site registration from the xcloud metrics driver.

   https://github.com/cs3org/reva/pull/1595

 * Enhancement #1506: Site Accounts service for API keys

   This update adds a new service to Reva that handles site accounts creation and management.
   Registered sites can be assigned an API key through a simple web interface which is also part of
   this service. This API key can then be used to identify a user and his/her associated (vendor or
   partner) site.

   Furthermore, Mentix was extended to make use of this new service. This way, all sites now have a
   stable and unique site ID that not only avoids ID collisions but also introduces a new layer of
   security (i.e., sites can only be modified or removed using the correct API key).

   https://github.com/cs3org/reva/pull/1506

 * Enhancement #116: Enhance storage registry with virtual views and regular expressions

   Add the functionality to the storage registry service to handle user requests for references
   which can span across multiple storage providers, particularly useful for cases where
   directories are sharded across providers or virtual views are expected.

   https://github.com/cs3org/cs3apis/pull/116
   https://github.com/cs3org/reva/pull/1570

 * Enhancement #1513: Add stubs for storage spaces manipulation

   This PR adds stubs for the storage space CRUD methods in the storageprovider service and makes
   the expired shares janitor configureable in the publicshares SQL driver.

   https://github.com/cs3org/reva/pull/1513


