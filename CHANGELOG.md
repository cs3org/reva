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


Changelog for reva 1.5.1 (2021-01-19)
=======================================

The following sections list the changes in reva 1.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1401: Use the user in request for deciding the layout for non-home DAV requests
 * Fix #1413: Re-include the '.git' dir in the Docker images to pass the version tag
 * Fix #1399: Fix ocis trash-bin purge
 * Enh #1397: Bump the Copyright date to 2021
 * Enh #1398: Support site authorization status in Mentix
 * Enh #1393: Allow setting favorites, mtime and a temporary etag
 * Enh #1403: Support remote cloud gathering metrics

Details
-------

 * Bugfix #1401: Use the user in request for deciding the layout for non-home DAV requests

   For the incoming /dav/files/userID requests, we have different namespaces depending on
   whether the request is for the logged-in user's namespace or not. Since in the storage drivers,
   we specify the layout depending only on the user whose resources are to be accessed, this fails
   when a user wants to access another user's namespace when the storage provider depends on the
   logged in user's namespace. This PR fixes that.

   For example, consider the following case. The owncloud fs uses a layout {{substr 0 1
   .Id.OpaqueId}}/{{.Id.OpaqueId}}. The user einstein sends a request to access a resource
   shared with him, say /dav/files/marie/abcd, which should be allowed. However, based on the
   way we applied the layout, there's no way in which this can be translated to /m/marie/.

   https://github.com/cs3org/reva/pull/1401

 * Bugfix #1413: Re-include the '.git' dir in the Docker images to pass the version tag

   And git SHA to the release tool.

   https://github.com/cs3org/reva/pull/1413

 * Bugfix #1399: Fix ocis trash-bin purge

   Fixes the empty trash-bin functionality for ocis-storage

   https://github.com/owncloud/product/issues/254
   https://github.com/cs3org/reva/pull/1399

 * Enhancement #1397: Bump the Copyright date to 2021

   https://github.com/cs3org/reva/pull/1397

 * Enhancement #1398: Support site authorization status in Mentix

   This enhancement adds support for a site authorization status to Mentix. This way, sites
   registered via a web app can now be excluded until authorized manually by an administrator.

   Furthermore, Mentix now sets the scheme for Prometheus targets. This allows us to also support
   monitoring of sites that do not support the default HTTPS scheme.

   https://github.com/cs3org/reva/pull/1398

 * Enhancement #1393: Allow setting favorites, mtime and a temporary etag

   We now let the ocis driver persist favorites, set temporary etags and the mtime as arbitrary
   metadata.

   https://github.com/owncloud/ocis/issues/567
   https://github.com/cs3org/reva/issues/1394
   https://github.com/cs3org/reva/pull/1393

 * Enhancement #1403: Support remote cloud gathering metrics

   The current metrics package can only gather metrics either from json files. With this feature,
   the metrics can be gathered polling the http endpoints exposed by the owncloud/nextcloud
   sciencemesh apps.

   https://github.com/cs3org/reva/pull/1403


Changelog for reva 1.5.0 (2021-01-12)
=======================================

The following sections list the changes in reva 1.5.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1385: Run changelog check only if there are changes in the code
 * Fix #1333: Delete sdk unit tests
 * Fix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files
 * Fix #1338: Fix fd leaks
 * Fix #1343: Fix ocis move
 * Fix #551: Fix purging deleted files with the ocis storage
 * Fix #863: Fix dav api for trashbin
 * Fix #204: Fix the ocs share with me response
 * Fix #1351: Fix xattr.Remove error check for macOS
 * Fix #1320: Do not panic on remote.php/dav/files/
 * Fix #1379: Make Jaeger agent usable
 * Fix #1331: Fix capabilities response for multiple client versions
 * Fix #1281: When sharing via ocs look up user by username
 * Fix #1334: Handle removal of public shares by token or ID
 * Chg #990: Replace the user uuid with the username in ocs share responses
 * Enh #1350: Add auth protocol based on user agent
 * Enh #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors
 * Enh #1364: Remove expired Link on Get
 * Enh #1340: Add cache to store UID to UserID mapping in EOS
 * Enh #1154: Add support for the protobuf interface to eos metadata
 * Enh #1154: Merge-rebase from master 10/11/2020
 * Enh #1359: Add cache for calculated etags for home and shares directory
 * Enh #1321: Add support for multiple data transfer protocols
 * Enh #1324: Log expected errors with debug level
 * Enh #1351: Map errtypes to status
 * Enh #1347: Support property to enable health checking on a service
 * Enh #1332: Add import support to Mentix
 * Enh #1371: Use self-hosted Drone CI
 * Enh #1354: Map bad request and unimplement to http status codes
 * Enh #929: Include share types in ocs propfind responses
 * Enh #1328: Add CLI commands for public shares
 * Enh #1388: Support range header in GET requests
 * Enh #1361: Remove expired Link on Access
 * Enh #1386: Docker image for cs3org/revad:VERSION-eos
 * Enh #1368: Calculate and expose actual file permission set

Details
-------

 * Bugfix #1385: Run changelog check only if there are changes in the code

   https://github.com/cs3org/reva/pull/1385

 * Bugfix #1333: Delete sdk unit tests

   These depend on a remote server running reva and thus fail in case of version mismatches.

   https://github.com/cs3org/reva/pull/1333

 * Bugfix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files

   There was a regression in which we were not routing correctly to the right storage depending on
   the url.

   https://github.com/cs3org/reva/pull/1342

 * Bugfix #1338: Fix fd leaks

   There were some left over open file descriptors on simple.go.

   https://github.com/cs3org/reva/pull/1338

 * Bugfix #1343: Fix ocis move

   Use the old node id to build the target path for xattr updates.

   https://github.com/owncloud/ocis/issues/975
   https://github.com/cs3org/reva/pull/1343

 * Bugfix #551: Fix purging deleted files with the ocis storage

   The ocis storage could load the owner information of a deleted file. This caused the storage to
   not be able to purge deleted files.

   https://github.com/owncloud/ocis/issues/551

 * Bugfix #863: Fix dav api for trashbin

   The api was comparing the requested username to the userid.

   https://github.com/owncloud/ocis/issues/863

 * Bugfix #204: Fix the ocs share with me response

   The path of the files shared with me was incorrect.

   https://github.com/owncloud/product/issues/204
   https://github.com/cs3org/reva/pull/1346

 * Bugfix #1351: Fix xattr.Remove error check for macOS

   Previously, we checked the xattr.Remove error only for linux systems. Now macOS is checked
   also

   https://github.com/cs3org/reva/pull/1351

 * Bugfix #1320: Do not panic on remote.php/dav/files/

   Currently requests to /remote.php/dav/files/ result in panics since we cannot longer strip
   the user + destination from the url. This fixes the server response code and adds an error body to
   the response.

   https://github.com/cs3org/reva/pull/1320

 * Bugfix #1379: Make Jaeger agent usable

   Previously, you could not use tracing with jaeger agent because the tracing connector is
   always used instead of the tracing endpoint.

   This PR removes the defaults for collector and tracing endpoint.

   https://github.com/cs3org/reva/pull/1379

 * Bugfix #1331: Fix capabilities response for multiple client versions

   https://github.com/cs3org/reva/pull/1331

 * Bugfix #1281: When sharing via ocs look up user by username

   The ocs api returns usernames when listing share recipients, so the lookup when creating the
   share needs to search the usernames and not the userid.

   https://github.com/cs3org/reva/pull/1281

 * Bugfix #1334: Handle removal of public shares by token or ID

   Previously different drivers handled removing public shares using different means, either
   the token or the ID. Now, both the drivers support both these methods.

   https://github.com/cs3org/reva/pull/1334

 * Change #990: Replace the user uuid with the username in ocs share responses

   The ocs api should not send the users uuid. Replaced the uuid with the username.

   https://github.com/owncloud/ocis/issues/990
   https://github.com/cs3org/reva/pull/1375

 * Enhancement #1350: Add auth protocol based on user agent

   Previously, all available credential challenges are given to the client, for example, basic
   auth, bearer token, etc ... Different clients have different priorities to use one method or
   another, and before it was not possible to force a client to use one method without having a side
   effect on other clients.

   This PR adds the functionality to target a specific auth protocol based on the user agent HTTP
   header.

   https://github.com/cs3org/reva/pull/1350

 * Enhancement #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors

   Prevent the errors while storing new 'daily' releases from going unnoticed on the CI.

   https://github.com/cs3org/reva/pull/1362

 * Enhancement #1364: Remove expired Link on Get

   There is the scenario in which a public link has expired but ListPublicLink has not run,
   accessing a technically expired public share is still possible.

   https://github.com/cs3org/reva/pull/1364

 * Enhancement #1340: Add cache to store UID to UserID mapping in EOS

   Previously, we used to send an RPC to the user provider service for every lookup of user IDs from
   the UID stored in EOS. This PR adds an in-memory lock-protected cache to store this mapping.

   https://github.com/cs3org/reva/pull/1340

 * Enhancement #1154: Add support for the protobuf interface to eos metadata

   https://github.com/cs3org/reva/pull/1154

 * Enhancement #1154: Merge-rebase from master 10/11/2020

   https://github.com/cs3org/reva/pull/1154

 * Enhancement #1359: Add cache for calculated etags for home and shares directory

   Since we store the references in the shares directory instead of actual resources, we need to
   calculate the etag on every list/stat call. This is rather expensive so adding a cache would
   help to a great extent with regard to the performance.

   https://github.com/cs3org/reva/pull/1359

 * Enhancement #1321: Add support for multiple data transfer protocols

   Previously, we had to configure which data transfer protocol to use in the dataprovider
   service. A previous PR added the functionality to redirect requests to different handlers
   based on the request method but that would lead to conflicts if multiple protocols don't
   support mutually exclusive sets of requests. This PR adds the functionality to have multiple
   such handlers simultaneously and the client can choose which protocol to use.

   https://github.com/cs3org/reva/pull/1321
   https://github.com/cs3org/reva/pull/1285/

 * Enhancement #1324: Log expected errors with debug level

   While trying to download a non existing file and reading a non existing attribute are
   technically an error they are to be expected and nothing an admin can or even should act upon.

   https://github.com/cs3org/reva/pull/1324

 * Enhancement #1351: Map errtypes to status

   When mapping errtypes to grpc statuses we now also map bad request and not implemented /
   unsupported cases in the gateway storageprovider.

   https://github.com/cs3org/reva/pull/1351

 * Enhancement #1347: Support property to enable health checking on a service

   This update introduces a new service property called `ENABLE_HEALTH_CHECKS` that must be
   explicitly set to `true` if a service should be checked for its health status. This allows us to
   only enable these checks for partner sites only, skipping vendor sites.

   https://github.com/cs3org/reva/pull/1347

 * Enhancement #1332: Add import support to Mentix

   This update adds import support to Mentix, transforming it into a **Mesh Entity Exchanger**.
   To properly support vendor site management, a new connector that works on a local file has been
   added as well.

   https://github.com/cs3org/reva/pull/1332

 * Enhancement #1371: Use self-hosted Drone CI

   Previously, we used the drone cloud to run the CI for the project. Due to unexpected and sudden
   stop of the service for the cs3org we decided to self-host it.

   https://github.com/cs3org/reva/pull/1371

 * Enhancement #1354: Map bad request and unimplement to http status codes

   We now return a 400 bad request when a grpc call fails with an invalid argument status and a 501 not
   implemented when it fails with an unimplemented status. This prevents 500 errors when a user
   tries to add resources to the Share folder or a storage does not implement an action.

   https://github.com/cs3org/reva/pull/1354

 * Enhancement #929: Include share types in ocs propfind responses

   Added the share types to the ocs propfind response when a resource has been shared.

   https://github.com/owncloud/ocis/issues/929
   https://github.com/cs3org/reva/pull/1329

 * Enhancement #1328: Add CLI commands for public shares

   https://github.com/cs3org/reva/pull/1328

 * Enhancement #1388: Support range header in GET requests

   To allow resuming a download we now support GET requests with a range header.

   https://github.com/owncloud/ocis-reva/issues/12
   https://github.com/cs3org/reva/pull/1388

 * Enhancement #1361: Remove expired Link on Access

   Since there is no background jobs scheduled to wipe out expired resources, for the time being
   public links are going to be removed on a "on demand" basis, meaning whenever there is an API call
   that access the list of shares for a given resource, we will check whether the share is expired
   and delete it if so.

   https://github.com/cs3org/reva/pull/1361

 * Enhancement #1386: Docker image for cs3org/revad:VERSION-eos

   Based on eos:c8_4.8.15 (Centos8, version 4.8.15). To be used when the Reva daemon needs IPC
   with xrootd/eos via stdin/out.

   https://github.com/cs3org/reva/pull/1386
   https://github.com/cs3org/reva/pull/1389

 * Enhancement #1368: Calculate and expose actual file permission set

   Instead of hardcoding the permissions set for every file and folder to ListContainer:true,
   CreateContainer:true and always reporting the hardcoded string WCKDNVR for the WebDAV
   permissions we now aggregate the actual cs3 resource permission set in the storage drivers and
   correctly map them to ocs permissions and webdav permissions using a common role struct that
   encapsulates the mapping logic.

   https://github.com/owncloud/ocis/issues/552
   https://github.com/owncloud/ocis/issues/762
   https://github.com/owncloud/ocis/issues/763
   https://github.com/owncloud/ocis/issues/893
   https://github.com/owncloud/ocis/issues/1126
   https://github.com/owncloud/ocis-reva/issues/47
   https://github.com/owncloud/ocis-reva/issues/315
   https://github.com/owncloud/ocis-reva/issues/316
   https://github.com/owncloud/product/issues/270
   https://github.com/cs3org/reva/pull/1368


Changelog for reva 1.4.0 (2020-11-17)
=======================================

The following sections list the changes in reva 1.4.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1316: Fix listing shares for nonexisting path
 * Fix #1274: Let the gateway filter invalid references
 * Fix #1269: Handle more eos errors
 * Fix #1297: Check the err and the response status code
 * Fix #1260: Fix file descriptor leak on ocdav put handler
 * Fix #1253: Upload file to storage provider after assembling chunks
 * Fix #1264: Fix etag propagation in ocis driver
 * Fix #1255: Check current node when iterating over path segments
 * Fix #1265: Stop setting propagation xattr on new files
 * Fix #260: Filter share with me requests
 * Fix #1317: Prevent nil pointer when listing shares
 * Fix #1259: Fix propfind response code on forbidden files
 * Fix #1294: Fix error type in read node when file was not found
 * Fix #1258: Update share grants on share update
 * Enh #1257: Add a test user to all sites
 * Enh #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect
 * Enh #1239: Add logic for finding groups to user provider service
 * Enh #1280: Add a Reva SDK
 * Enh #1237: Setup of grpc transfer service and cli
 * Enh #1224: Add SQL driver for share manager
 * Enh #1285: Refactor the uploading files workflow from various clients
 * Enh #1233: Add support for custom CodiMD mimetype

Details
-------

 * Bugfix #1316: Fix listing shares for nonexisting path

   When trying to list shares for a not existing file or folder the ocs sharing implementation no
   longer responds with the wrong status code and broken xml.

   https://github.com/cs3org/reva/pull/1316

 * Bugfix #1274: Let the gateway filter invalid references

   We now filter deleted and unshared entries from the response when listing the shares folder of a
   user.

   https://github.com/cs3org/reva/pull/1274

 * Bugfix #1269: Handle more eos errors

   We now treat E2BIG, EACCES as a permission error, which occur, eg. when acl checks fail and
   return a permission denied error.

   https://github.com/cs3org/reva/pull/1269

 * Bugfix #1297: Check the err and the response status code

   The publicfile handler needs to check the response status code to return proper not pound and
   permission errors in the webdav api.

   https://github.com/cs3org/reva/pull/1297

 * Bugfix #1260: Fix file descriptor leak on ocdav put handler

   File descriptors on the ocdav service, especially on the put handler was leaking http
   connections. This PR addresses this.

   https://github.com/cs3org/reva/pull/1260

 * Bugfix #1253: Upload file to storage provider after assembling chunks

   In the PUT handler for chunked uploads in ocdav, we store the individual chunks in temporary
   file but do not write the assembled file to storage. This PR fixes that.

   https://github.com/cs3org/reva/pull/1253

 * Bugfix #1264: Fix etag propagation in ocis driver

   We now use a new synctime timestamp instead of trying to read the mtime to avoid race conditions
   when the stat request happens too quickly.

   https://github.com/owncloud/product/issues/249
   https://github.com/cs3org/reva/pull/1264

 * Bugfix #1255: Check current node when iterating over path segments

   When checking permissions we were always checking the leaf instead of using the current node
   while iterating over path segments.

   https://github.com/cs3org/reva/pull/1255

 * Bugfix #1265: Stop setting propagation xattr on new files

   We no longer set the propagation flag on a file because it is only evaluated for folders anyway.

   https://github.com/cs3org/reva/pull/1265

 * Bugfix #260: Filter share with me requests

   The OCS API now properly filters share with me requests by path and by share status (pending,
   accepted, rejected, all)

   https://github.com/owncloud/ocis-reva/issues/260
   https://github.com/owncloud/ocis-reva/issues/311
   https://github.com/cs3org/reva/pull/1301

 * Bugfix #1317: Prevent nil pointer when listing shares

   We now handle cases where the grpc connection failed correctly by no longer trying to access the
   response status.

   https://github.com/cs3org/reva/pull/1317

 * Bugfix #1259: Fix propfind response code on forbidden files

   When executing a propfind to a resource owned by another user the service would respond with a
   HTTP 403. In ownCloud 10 the response was HTTP 207. This change sets the response code to HTTP 207
   to stay backwards compatible.

   https://github.com/cs3org/reva/pull/1259

 * Bugfix #1294: Fix error type in read node when file was not found

   The method ReadNode in the ocis storage didn't return the error type NotFound when a file was not
   found.

   https://github.com/cs3org/reva/pull/1294

 * Bugfix #1258: Update share grants on share update

   When a share was updated the share information in the share manager was updated but the grants
   set by the storage provider were not.

   https://github.com/cs3org/reva/pull/1258

 * Enhancement #1257: Add a test user to all sites

   For health monitoring of all mesh sites, we need a special user account that is present on every
   site. This PR adds such a user to each users-*.json file so that every site will have the same test
   user credentials.

   https://github.com/cs3org/reva/pull/1257

 * Enhancement #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect

   Applications served by the WOPI bridge (CodiMD for the time being) require an extra
   redirection as the WOPI bridge itself behaves like a user app. This change returns to the client
   the redirected URL from the WOPI bridge, which is the real application URL.

   https://github.com/cs3org/reva/pull/1234

 * Enhancement #1239: Add logic for finding groups to user provider service

   To create shares with user groups, the functionality for searching for these based on a pattern
   is needed. This PR adds that.

   https://github.com/cs3org/reva/pull/1239

 * Enhancement #1280: Add a Reva SDK

   A Reva SDK has been added to make working with a remote Reva instance much easier by offering a
   high-level API that hides all the underlying details of the CS3API.

   https://github.com/cs3org/reva/pull/1280

 * Enhancement #1237: Setup of grpc transfer service and cli

   The grpc transfer service and cli for it.

   https://github.com/cs3org/reva/pull/1237

 * Enhancement #1224: Add SQL driver for share manager

   This PR adds an SQL driver for the shares manager which expects a schema equivalent to the one
   used in production for CERNBox.

   https://github.com/cs3org/reva/pull/1224

 * Enhancement #1285: Refactor the uploading files workflow from various clients

   Previously, we were implementing the tus client logic in the ocdav service, leading to
   restricting the whole of tus logic to the internal services. This PR refactors that workflow to
   accept incoming requests following the tus protocol while using simpler transmission
   internally.

   https://github.com/cs3org/reva/pull/1285
   https://github.com/cs3org/reva/pull/1314

 * Enhancement #1233: Add support for custom CodiMD mimetype

   The new mimetype is associated with the `.zmd` file extension. The corresponding
   configuration is associated with the storageprovider.

   https://github.com/cs3org/reva/pull/1233
   https://github.com/cs3org/reva/pull/1284


Changelog for reva 1.3.0 (2020-10-08)
=======================================

The following sections list the changes in reva 1.3.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1140: Call the gateway stat method from appprovider
 * Fix #1170: Up and download of file shares
 * Fix #1177: Fix ocis move
 * Fix #1178: Fix litmus failing on ocis storage
 * Fix #237: Fix missing quotes on OCIS-Storage
 * Fix #1210: No longer swallow permissions errors in the gateway
 * Fix #1183: Handle eos EPERM as permission denied
 * Fix #1206: No longer swallow permissions errors
 * Fix #1207: No longer swallow permissions errors in ocdav
 * Fix #1161: Cache display names in ocs service
 * Fix #1216: Add error handling for invalid references
 * Enh #1205: Allow using the username when accessing the users home
 * Enh #1131: Use updated cato to display nested package config in parent docs
 * Enh #1213: Check permissions in ocis driver
 * Enh #1202: Check permissions in owncloud driver
 * Enh #1228: Add GRPC stubs for CreateSymlink method
 * Enh #1174: Add logic in EOS FS for maintaining same inode across file versions
 * Enh #1142: Functionality to map home directory to different storage providers
 * Enh #1190: Add Blackbox Exporter support to Mentix
 * Enh #1229: New gateway datatx service
 * Enh #1225: Allow setting the owner when using the ocis driver
 * Enh #1180: Introduce ocis driver treetime accounting
 * Enh #1208: Calculate etags on-the-fly for shares directory and home folder

Details
-------

 * Bugfix #1140: Call the gateway stat method from appprovider

   The appprovider service used to directly pass the stat request to the storage provider
   bypassing the gateway, which resulted in errors while handling share children as they are
   resolved in the gateway path.

   https://github.com/cs3org/reva/pull/1140

 * Bugfix #1170: Up and download of file shares

   The shared folder logic in the gateway storageprovider was not allowing file up and downloads
   for single file shares. We now check if the reference is actually a file to determine if up /
   download should be allowed.

   https://github.com/cs3org/reva/pull/1170

 * Bugfix #1177: Fix ocis move

   When renaming a file we updating the name attribute on the wrong node, causing the path
   construction to use the wrong name. This fixes the litmus move_coll test.

   https://github.com/cs3org/reva/pull/1177

 * Bugfix #1178: Fix litmus failing on ocis storage

   We now ignore the `no data available` error when removing a non existing metadata attribute,
   which is ok because we are trying to delete it anyway.

   https://github.com/cs3org/reva/issues/1178
   https://github.com/cs3org/reva/pull/1179

 * Bugfix #237: Fix missing quotes on OCIS-Storage

   Etags have to be enclosed in quotes ". Return correct etags on OCIS-Storage.

   https://github.com/owncloud/product/issues/237
   https://github.com/cs3org/reva/pull/1232

 * Bugfix #1210: No longer swallow permissions errors in the gateway

   The gateway is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `status.NewPermissionDenied` or `status.NewInternal` instead of reusing the original
   response status.

   https://github.com/cs3org/reva/pull/1210

 * Bugfix #1183: Handle eos EPERM as permission denied

   We now treat EPERM errors, which occur, eg. when acl checks fail and return a permission denied
   error.

   https://github.com/cs3org/reva/pull/1183

 * Bugfix #1206: No longer swallow permissions errors

   The storageprovider is no longer ignoring permissions errors. It will now report them
   properly using `status.NewPermissionDenied(...)` instead of `status.NewInternal(...)`

   https://github.com/cs3org/reva/pull/1206

 * Bugfix #1207: No longer swallow permissions errors in ocdav

   The ocdav api is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `http.StatusForbidden` instead of `http.StatusInternalServerError`

   https://github.com/cs3org/reva/pull/1207

 * Bugfix #1161: Cache display names in ocs service

   The ocs list shares endpoint may need to fetch the displayname for multiple different users. We
   are now caching the lookup fo 60 seconds to save redundant RPCs to the users service.

   https://github.com/cs3org/reva/pull/1161

 * Bugfix #1216: Add error handling for invalid references

   https://github.com/cs3org/reva/pull/1216
   https://github.com/cs3org/reva/pull/1218

 * Enhancement #1205: Allow using the username when accessing the users home

   We now allow using the userid and the username when accessing the users home on the `/dev/files`
   endpoint.

   https://github.com/cs3org/reva/pull/1205

 * Enhancement #1131: Use updated cato to display nested package config in parent docs

   Previously, in case of nested packages, we just had a link pointing to the child package. Now we
   copy the nested package's documentation to the parent itself to make it easier for devs.

   https://github.com/cs3org/reva/pull/1131

 * Enhancement #1213: Check permissions in ocis driver

   We are now checking grant permissions in the ocis storage driver.

   https://github.com/cs3org/reva/pull/1213

 * Enhancement #1202: Check permissions in owncloud driver

   We are now checking grant permissions in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1202

 * Enhancement #1228: Add GRPC stubs for CreateSymlink method

   https://github.com/cs3org/reva/pull/1228

 * Enhancement #1174: Add logic in EOS FS for maintaining same inode across file versions

   This PR adds the functionality to maintain the same inode across various versions of a file by
   returning the inode of the version folder which remains constant. It requires extra metadata
   operations so a flag is provided to disable it.

   https://github.com/cs3org/reva/pull/1174

 * Enhancement #1142: Functionality to map home directory to different storage providers

   We hardcode the home path for all users to /home. This forbids redirecting requests for
   different users to multiple storage providers. This PR provides the option to map the home
   directories of different users using user attributes.

   https://github.com/cs3org/reva/pull/1142

 * Enhancement #1190: Add Blackbox Exporter support to Mentix

   This update extends Mentix to export a Prometheus SD file specific to the Blackbox Exporter
   which will be used for initial health monitoring. Usually, Prometheus requires its targets to
   only consist of the target's hostname; the BBE though expects a full URL here. This makes
   exporting two distinct files necessary.

   https://github.com/cs3org/reva/pull/1190

 * Enhancement #1229: New gateway datatx service

   Represents the CS3 datatx module in the gateway.

   https://github.com/cs3org/reva/pull/1229

 * Enhancement #1225: Allow setting the owner when using the ocis driver

   To support the metadata storage we allow setting the owner of the root node so that subsequent
   requests with that owner can be used to manage the storage.

   https://github.com/cs3org/reva/pull/1225

 * Enhancement #1180: Introduce ocis driver treetime accounting

   We added tree time accounting to the ocis storage driver which is modeled after [eos synctime
   accounting](http://eos-docs.web.cern.ch/eos-docs/configuration/namespace.html#enable-subtree-accounting).
   It can be enabled using the new `treetime_accounting` option, which defaults to `false` The
   `tmtime` is stored in an extended attribute `user.ocis.tmtime`. The treetime accounting is
   enabled for nodes which have the `user.ocis.propagation` extended attribute set to `"1"`.
   Currently, propagation is in sync.

   https://github.com/cs3org/reva/pull/1180

 * Enhancement #1208: Calculate etags on-the-fly for shares directory and home folder

   We create references for accepted shares in the shares directory, but these aren't updated
   when the original resource is modified. This PR adds the functionality to generate the etag for
   the shares directory and correspondingly, the home directory, based on the actual resources
   which the references point to, enabling the sync functionality.

   https://github.com/cs3org/reva/pull/1208


Changelog for reva 1.2.1 (2020-09-15)
=======================================

The following sections list the changes in reva 1.2.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1124: Do not swallow 'not found' errors in Stat
 * Enh #1125: Rewire dav files to the home storage
 * Enh #559: Introduce ocis storage driver
 * Enh #1118: Metrics module can be configured to retrieve metrics data from file

Details
-------

 * Bugfix #1124: Do not swallow 'not found' errors in Stat

   Webdav needs to determine if a file exists to return 204 or 201 response codes. When stating a non
   existing resource the NOT_FOUND code was replaced with an INTERNAL error code. This PR passes
   on a NOT_FOUND status code in the gateway.

   https://github.com/cs3org/reva/pull/1124

 * Enhancement #1125: Rewire dav files to the home storage

   If the user specified in the dav files URL matches the current one, rewire it to use the
   webDavHandler which is wired to the home storage.

   This fixes path mapping issues.

   https://github.com/cs3org/reva/pull/1125

 * Enhancement #559: Introduce ocis storage driver

   We introduced a now storage driver `ocis` that deconstructs a filesystem and uses a node first
   approach to implement an efficient lookup of files by path as well as by file id.

   https://github.com/cs3org/reva/pull/559

 * Enhancement #1118: Metrics module can be configured to retrieve metrics data from file

   - Export site metrics in Prometheus #698

   https://github.com/cs3org/reva/pull/1118


Changelog for reva 1.2.0 (2020-08-25)
=======================================

The following sections list the changes in reva 1.2.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1099: Do not restore recycle entry on purge
 * Fix #1091: Allow listing the trashbin
 * Fix #1103: Restore and delete trash items via ocs
 * Fix #1090: Ensure ignoring public stray shares
 * Fix #1064: Ensure ignoring stray shares
 * Fix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient
 * Fix #1115: Owncloud driver - propagate mtime on RemoveGrant
 * Fix #1111: Handle redirection prefixes when extracting destination from URL
 * Enh #1101: Add UID and GID in ldap auth driver
 * Enh #1077: Add calens check to verify changelog entries in CI
 * Enh #1072: Refactor Reva CLI with prompts
 * Enh #1079: Get file info using fxids from EOS
 * Enh #1088: Update LDAP user driver
 * Enh #1114: System information metrics cleanup
 * Enh #1071: System information included in Prometheus metrics
 * Enh #1094: Add logic for resolving storage references over webdav

Details
-------

 * Bugfix #1099: Do not restore recycle entry on purge

   This PR fixes a bug in the EOSFS driver that was restoring a deleted entry when asking for its
   permanent purge. EOS does not have the functionality to purge individual files, but the whole
   recycle of the user.

   https://github.com/cs3org/reva/pull/1099

 * Bugfix #1091: Allow listing the trashbin

   The trashbin endpoint expects the userid, not the username.

   https://github.com/cs3org/reva/pull/1091

 * Bugfix #1103: Restore and delete trash items via ocs

   The OCS api was not passing the correct key and references to the CS3 API. Furthermore, the
   owncloud storage driver was constructing the wrong target path when restoring.

   https://github.com/cs3org/reva/pull/1103

 * Bugfix #1090: Ensure ignoring public stray shares

   When using the json public shares manager, it can be the case we found a share with a resource_id
   that no longer exists.

   https://github.com/cs3org/reva/pull/1090

 * Bugfix #1064: Ensure ignoring stray shares

   When using the json shares manager, it can be the case we found a share with a resource_id that no
   longer exists. This PR aims to address his case by changing the contract of getPath and return
   the result of the STAT call instead of a generic error, so we can check the cause.

   https://github.com/cs3org/reva/pull/1064

 * Bugfix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient

   https://github.com/cs3org/reva/pull/1082
   https://github.com/cs3org/reva/pull/1116

 * Bugfix #1115: Owncloud driver - propagate mtime on RemoveGrant

   When removing a grant the mtime change also needs to be propagated. Only affectsn the owncluod
   storage driver.

   https://github.com/cs3org/reva/pull/1115

 * Bugfix #1111: Handle redirection prefixes when extracting destination from URL

   The move function handler in ocdav extracts the destination path from the URL by removing the
   base URL prefix from the URL path. This would fail in case there is a redirection prefix. This PR
   takes care of that and it also allows zero-size uploads for localfs.

   https://github.com/cs3org/reva/pull/1111

 * Enhancement #1101: Add UID and GID in ldap auth driver

   The PR https://github.com/cs3org/reva/pull/1088/ added the functionality to lookup UID
   and GID from the ldap user provider. This PR adds the same to the ldap auth manager.

   https://github.com/cs3org/reva/pull/1101

 * Enhancement #1077: Add calens check to verify changelog entries in CI

   https://github.com/cs3org/reva/pull/1077

 * Enhancement #1072: Refactor Reva CLI with prompts

   The current CLI is a bit cumbersome to use with users having to type commands containing all the
   associated config with no prompts or auto-completes. This PR refactors the CLI with these
   functionalities.

   https://github.com/cs3org/reva/pull/1072

 * Enhancement #1079: Get file info using fxids from EOS

   This PR supports getting file information from EOS using the fxid value.

   https://github.com/cs3org/reva/pull/1079

 * Enhancement #1088: Update LDAP user driver

   The LDAP user driver can now fetch users by a single claim / attribute. Use an `attributefilter`
   like `(&(objectclass=posixAccount)({{attr}}={{value}}))` in the driver section.

   It also adds the uid and gid to the users opaque properties so that eos can use them for chown and
   acl operations.

   https://github.com/cs3org/reva/pull/1088

 * Enhancement #1114: System information metrics cleanup

   The system information metrics are now based on OpenCensus instead of the Prometheus client
   library. Furthermore, its initialization was moved out of the Prometheus HTTP service to keep
   things clean.

   https://github.com/cs3org/reva/pull/1114

 * Enhancement #1071: System information included in Prometheus metrics

   System information is now included in the main Prometheus metrics exposed at `/metrics`.

   https://github.com/cs3org/reva/pull/1071

 * Enhancement #1094: Add logic for resolving storage references over webdav

   This PR adds the functionality to resolve webdav references using the ocs webdav service by
   passing the resource's owner's token. This would need to be changed in production.

   https://github.com/cs3org/reva/pull/1094


Changelog for reva 1.1.0 (2020-08-11)
=======================================

The following sections list the changes in reva 1.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1069: Pass build time variables while compiling
 * Fix #1047: Fix missing idp check in GetUser of demo userprovider
 * Fix #1038: Do not stat shared resources when downloading
 * Fix #1034: Fixed some error reporting strings and corresponding logs
 * Fix #1046: Fixed resolution of fileid in GetPathByID
 * Fix #1052: Ocfs: Lookup user to render template properly
 * Fix #1024: Take care of trailing slashes in OCM package
 * Fix #1025: Use lower-case name for changelog directory
 * Fix #1042: List public shares only created by the current user
 * Fix #1051: Disallow sharing the shares directory
 * Enh #1035: Refactor AppProvider workflow
 * Enh #1059: Improve timestamp precision while logging
 * Enh #1037: System information HTTP service
 * Enh #995: Add UID and GID to the user object from user package

Details
-------

 * Bugfix #1069: Pass build time variables while compiling

   We provide the option of viewing various configuration and version options in both reva CLI as
   well as the reva daemon, but we didn't actually have these values in the first place. This PR adds
   that info at compile time.

   https://github.com/cs3org/reva/pull/1069

 * Bugfix #1047: Fix missing idp check in GetUser of demo userprovider

   We've added a check for matching idp in the GetUser function of the demo userprovider

   https://github.com/cs3org/reva/issues/1047

 * Bugfix #1038: Do not stat shared resources when downloading

   Previously, we statted the resources in all download requests resulting in failures when
   downloading references. This PR fixes that by statting only in case the resource is not present
   in the shares folder. It also fixes a bug where we allowed uploading to the mount path, resulting
   in overwriting the user home directory.

   https://github.com/cs3org/reva/pull/1038

 * Bugfix #1034: Fixed some error reporting strings and corresponding logs

   https://github.com/cs3org/reva/pull/1034

 * Bugfix #1046: Fixed resolution of fileid in GetPathByID

   Following refactoring of fileid generations in the local storage provider, this ensures
   fileid to path resolution works again.

   https://github.com/cs3org/reva/pull/1046

 * Bugfix #1052: Ocfs: Lookup user to render template properly

   Currently, the username is used to construct paths, which breaks when mounting the `owncloud`
   storage driver at `/oc` and then expecting paths that use the username like
   `/oc/einstein/foo` to work, because they will mismatch the path that is used from propagation
   which uses `/oc/u-u-i-d` as the root, giving an `internal path outside root` error

   https://github.com/cs3org/reva/pull/1052

 * Bugfix #1024: Take care of trailing slashes in OCM package

   Previously, we assumed that the OCM endpoints would have trailing slashes, failing in case
   they didn't. This PR fixes that.

   https://github.com/cs3org/reva/pull/1024

 * Bugfix #1025: Use lower-case name for changelog directory

   When preparing a new release, the changelog entries need to be copied to the changelog folder
   under docs. In a previous change, all these folders were made to have lower case names,
   resulting in creation of a separate folder.

   https://github.com/cs3org/reva/pull/1025

 * Bugfix #1042: List public shares only created by the current user

   When running ocis, the public links created by a user are visible to all the users under the
   'Shared with others' tab. This PR fixes that by returning only those links which are created by a
   user themselves.

   https://github.com/cs3org/reva/pull/1042

 * Bugfix #1051: Disallow sharing the shares directory

   Previously, it was possible to create public links for and share the shares directory itself.
   However, when the recipient tried to accept the share, it failed. This PR prevents the creation
   of such shares in the first place.

   https://github.com/cs3org/reva/pull/1051

 * Enhancement #1035: Refactor AppProvider workflow

   Simplified the app-provider configuration: storageID is worked out automatically and UIURL
   is suppressed for now. Implemented the new gRPC protocol from the gateway to the appprovider.

   https://github.com/cs3org/reva/pull/1035

 * Enhancement #1059: Improve timestamp precision while logging

   Previously, the timestamp associated with a log just had the hour and minute, which made
   debugging quite difficult. This PR increases the precision of the associated timestamp.

   https://github.com/cs3org/reva/pull/1059

 * Enhancement #1037: System information HTTP service

   This service exposes system information via an HTTP endpoint. This currently only includes
   Reva version information but can be extended easily. The information are exposed in the form of
   Prometheus metrics so that we can gather these in a streamlined way.

   https://github.com/cs3org/reva/pull/1037

 * Enhancement #995: Add UID and GID to the user object from user package

   Currently, the UID and GID for users need to be read from the local system which requires local
   users to be present. This change retrieves that information from the user and auth packages and
   adds methods to retrieve it.

   https://github.com/cs3org/reva/pull/995


Changelog for reva 1.0.0 (2020-07-28)
=======================================

The following sections list the changes in reva 1.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #941: Fix initialization of json share manager
 * Fix #1006: Check if SMTP credentials are nil
 * Chg #965: Remove protocol from partner domains to match gocdb config
 * Enh #986: Added signing key capability
 * Enh #922: Add tutorial for deploying WOPI and Reva locally
 * Enh #979: Skip changelog enforcement for bot PRs
 * Enh #965: Enforce adding changelog in make and CI
 * Enh #1016: Do not enforce changelog on release
 * Enh #969: Allow requests to hosts with unverified certificates
 * Enh #914: Make httpclient configurable
 * Enh #972: Added a site locations exporter to Mentix
 * Enh #1000: Forward share invites to the provider selected in meshdirectory
 * Enh #1002: Pass the link to the meshdirectory service in token mail
 * Enh #1008: Use proper logging for ldap auth requests
 * Enh #970: Add required headers to SMTP client to prevent being tagged as spam
 * Enh #996: Split LDAP user filters
 * Enh #1007: Update go-tus version
 * Enh #1004: Update github.com/go-ldap/ldap to v3
 * Enh #974: Add functionality to create webdav references for OCM shares

Details
-------

 * Bugfix #941: Fix initialization of json share manager

   When an empty shares.json file existed the json share manager would fail while trying to
   unmarshal the empty file.

   https://github.com/cs3org/reva/issues/941
   https://github.com/cs3org/reva/pull/940

 * Bugfix #1006: Check if SMTP credentials are nil

   Check if SMTP credentials are nil before passing them to the SMTPClient, causing it to crash.

   https://github.com/cs3org/reva/pull/1006

 * Change #965: Remove protocol from partner domains to match gocdb config

   Minor changes for OCM cross-partner testing.

   https://github.com/cs3org/reva/pull/965

 * Enhancement #986: Added signing key capability

   The ocs capabilities can now hold the boolean flag to indicate url signing endpoint and
   middleware are available

   https://github.com/cs3org/reva/pull/986

 * Enhancement #922: Add tutorial for deploying WOPI and Reva locally

   Add a new tutorial on how to run Reva and Wopiserver together locally

   https://github.com/cs3org/reva/pull/922

 * Enhancement #979: Skip changelog enforcement for bot PRs

   Skip changelog enforcement for bot PRs.

   https://github.com/cs3org/reva/pull/979

 * Enhancement #965: Enforce adding changelog in make and CI

   When adding a feature or fixing a bug, a changelog needs to be specified, failing which the build
   wouldn't pass.

   https://github.com/cs3org/reva/pull/965

 * Enhancement #1016: Do not enforce changelog on release

   While releasing a new version of Reva, make release was failing because it was enforcing a
   changelog entry.

   https://github.com/cs3org/reva/pull/1016

 * Enhancement #969: Allow requests to hosts with unverified certificates

   Allow OCM to send requests to other mesh providers with the option of skipping certificate
   verification.

   https://github.com/cs3org/reva/pull/969

 * Enhancement #914: Make httpclient configurable

   - Introduce Options for the httpclient (#914)

   https://github.com/cs3org/reva/pull/914

 * Enhancement #972: Added a site locations exporter to Mentix

   Mentix now offers an endpoint that exposes location information of all sites in the mesh. This
   can be used in Grafana's world map view to show the exact location of every site.

   https://github.com/cs3org/reva/pull/972

 * Enhancement #1000: Forward share invites to the provider selected in meshdirectory

   Added a share invite forward OCM endpoint to the provider links (generated when a user picks a
   target provider in the meshdirectory service web interface), together with an invitation
   token and originating provider domain passed to the service via query params.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1000

 * Enhancement #1002: Pass the link to the meshdirectory service in token mail

   Currently, we just forward the token and the original user's domain when forwarding an OCM
   invite token and expect the user to frame the forward invite URL. This PR instead passes the link
   to the meshdirectory service, from where the user can pick the provider they want to accept the
   invite with.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1002

 * Enhancement #1008: Use proper logging for ldap auth requests

   Instead of logging to stdout we now log using debug level logging or error level logging in case
   the configured system user cannot bind to LDAP.

   https://github.com/cs3org/reva/pull/1008

 * Enhancement #970: Add required headers to SMTP client to prevent being tagged as spam

   Mails being sent through the client, specially through unauthenticated SMTP were being
   tagged as spam due to missing headers.

   https://github.com/cs3org/reva/pull/970

 * Enhancement #996: Split LDAP user filters

   The current LDAP user and auth filters only allow a single `%s` to be replaced with the relevant
   string. The current `userfilter` is used to lookup a single user, search for share recipients
   and for login. To make each use case more flexible we split this in three and introduced
   templates.

   For the `userfilter` we moved to filter templates that can use the CS3 user id properties
   `{{.OpaqueId}}` and `{{.Idp}}`: ``` userfilter =
   "(&(objectclass=posixAccount)(|(ownclouduuid={{.OpaqueId}})(cn={{.OpaqueId}})))"
   ```

   We introduced a new `findfilter` that is used when searching for users. Use it like this: ```
   findfilter =
   "(&(objectclass=posixAccount)(|(cn={{query}}*)(displayname={{query}}*)(mail={{query}}*)))"
   ```

   Furthermore, we also introduced a dedicated login filter for the LDAP auth manager: ```
   loginfilter = "(&(objectclass=posixAccount)(|(cn={{login}})(mail={{login}})))" ```

   These filter changes are backward compatible: `findfilter` and `loginfilter` will be
   derived from the `userfilter` by replacing `%s` with `{{query}}` and `{{login}}`
   respectively. The `userfilter` replaces `%s` with `{{.OpaqueId}}`

   Finally, we changed the default attribute for the immutable uid of a user to
   `ms-DS-ConsistencyGuid`. See
   https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
   for the background. You can fall back to `objectguid` or even `samaccountname` but you will run
   into trouble when user names change. You have been warned.

   https://github.com/cs3org/reva/pull/996

 * Enhancement #1007: Update go-tus version

   The lib now uses go mod which should help golang to sort out dependencies when running `go mod
   tidy`.

   https://github.com/cs3org/reva/pull/1007

 * Enhancement #1004: Update github.com/go-ldap/ldap to v3

   In the current version of the ldap lib attribute comparisons are case sensitive. With v3
   `GetEqualFoldAttributeValue` is introduced, which allows a case insensitive comparison.
   Which AFAICT is what the spec says: see
   https://github.com/go-ldap/ldap/issues/129#issuecomment-333744641

   https://github.com/cs3org/reva/pull/1004

 * Enhancement #974: Add functionality to create webdav references for OCM shares

   Webdav references will now be created in users' shares directory with the target set to the
   original resource's location in their mesh provider.

   https://github.com/cs3org/reva/pull/974


Changelog for reva 0.1.0 (2020-03-18)
=======================================

The following sections list the changes in reva 0.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Enh #402: Build daily releases
 * Enh #416: Improve developer experience
 * Enh #468: remove vendor support
 * Enh #545: simplify configuration
 * Enh #561: improve the documentation
 * Enh #562: support home storages

Details
-------

 * Enhancement #402: Build daily releases

   Reva was not building releases of commits to the master branch. Thanks to @zazola.

   Commit-based released are generated every time a PR is merged into master. These releases are
   available at: https://reva-releases.web.cern.ch

   https://github.com/cs3org/reva/pull/402

 * Enhancement #416: Improve developer experience

   Reva provided the option to be run with a single configuration file by using the -c config flag.

   This PR adds the flag -dev-dir than can point to a directory containing multiple config files.
   The reva daemon will launch a new process per configuration file.

   Kudos to @refs.

   https://github.com/cs3org/reva/pull/416

 * Enhancement #468: remove vendor support

   Because @dependabot cannot update in a clean way the vendor dependecies Reva removed support
   for vendored dependencies inside the project.

   Dependencies will continue to be versioned but they will be downloaded when compiling the
   artefacts.

   https://github.com/cs3org/reva/pull/468
   https://github.com/cs3org/reva/pull/524

 * Enhancement #545: simplify configuration

   Reva configuration was difficul as many of the configuration parameters were not providing
   sane defaults. This PR and the related listed below simplify the configuration.

   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/536
   https://github.com/cs3org/reva/pull/568

 * Enhancement #561: improve the documentation

   Documentation has been improved and can be consulted here: https://reva.link

   https://github.com/cs3org/reva/pull/561
   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/568

 * Enhancement #562: support home storages

   Reva did not have any functionality to handle home storages. These PRs make that happen.

   https://github.com/cs3org/reva/pull/562
   https://github.com/cs3org/reva/pull/510
   https://github.com/cs3org/reva/pull/493
   https://github.com/cs3org/reva/pull/476
   https://github.com/cs3org/reva/pull/469
   https://github.com/cs3org/reva/pull/436
   https://github.com/cs3org/reva/pull/571


Changelog for reva 0.0.1 (2019-10-24)
=======================================

The following sections list the changes in reva 0.0.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Enh #334: Create release procedure for Reva

Details
-------

 * Enhancement #334: Create release procedure for Reva

   Reva did not have any procedure to release versions. This PR brings a new tool to release Reva
   versions (tools/release) and prepares the necessary files for artefact distributed made
   from Drone into Github pages.

   https://github.com/cs3org/reva/pull/334


