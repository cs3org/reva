
---
title: "v1.5.0"
linkTitle: "v1.5.0"
weight: 40
description: >
  Changelog for Reva v1.5.0 (2021-01-12)
---

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


