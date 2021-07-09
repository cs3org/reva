
---
title: "v1.9.1"
linkTitle: "v1.9.1"
weight: 40
description: >
  Changelog for Reva v1.9.1 (2021-07-09)
---

Changelog for reva 1.9.1 (2021-07-09)
=======================================

The following sections list the changes in reva 1.9.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder
 * Fix #1835: Cleanup owncloudsql driver
 * Fix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping
 * Fix #1885: Fix template in eoshomewrapper to use context user rather than resource
 * Fix #1833: Properly handle name collisions for deletes in the owncloud driver
 * Fix #1874: Use the original file mtime during upload
 * Fix #1854: Add the uid/gid to the url for eos
 * Fix #1848: Fill in missing gid/uid number with nobody
 * Fix #1831: Make the ocm-provider endpoint in the ocmd service unprotected
 * Fix #1808: Use empty array in OCS Notifications endpoints
 * Fix #1825: Raise max grpc message size
 * Fix #1828: Send a proper XML header with error messages
 * Chg #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7
 * Enh #1834: Add API key to Mentix GOCDB connector
 * Enh #1855: Minor optimization in parsing EOS ACLs
 * Enh #1873: Update the EOS image tag to be for revad-eos image
 * Enh #1802: Introduce list spaces
 * Enh #1849: Add readonly interceptor
 * Enh #1875: Simplify resource comparison
 * Enh #1827: Support trashbin sub paths in the recycle API

Details
-------

 * Bugfix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder

   This was introduced on https://github.com/cs3org/reva/commit/117adad while porting the
   configuration on .drone.yml to starlark.

   Force golang:alpine3.13 as base image to prevent errors from Make when running on Docker
   <20.10 as it happens on Drone
   ref.https://gitlab.alpinelinux.org/alpine/aports/-/issues/12396

   https://github.com/cs3org/reva/pull/1843
   https://github.com/cs3org/reva/pull/1844
   https://github.com/cs3org/reva/pull/1847

 * Bugfix #1835: Cleanup owncloudsql driver

   Use `owncloudsql` string when returning errors and removed copyMD as it does not need to copy
   metadata from files.

   https://github.com/cs3org/reva/pull/1835

 * Bugfix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping

   https://github.com/cs3org/reva/pull/1868

 * Bugfix #1885: Fix template in eoshomewrapper to use context user rather than resource

   https://github.com/cs3org/reva/pull/1885

 * Bugfix #1833: Properly handle name collisions for deletes in the owncloud driver

   In the owncloud storage driver when we delete a file we append the deletion time to the file name.
   If two fast consecutive deletes happened, the deletion time would be the same and if the two
   files had the same name we ended up with only one file in the trashbin.

   https://github.com/cs3org/reva/pull/1833

 * Bugfix #1874: Use the original file mtime during upload

   The decomposedfs was not using the original file mtime during uploads.

   https://github.com/cs3org/reva/pull/1874

 * Bugfix #1854: Add the uid/gid to the url for eos

   https://github.com/cs3org/reva/pull/1854

 * Bugfix #1848: Fill in missing gid/uid number with nobody

   When an LDAP server does not provide numeric uid or gid properties for a user we now fall back to a
   configurable `nobody` id (default 99).

   https://github.com/cs3org/reva/pull/1848

 * Bugfix #1831: Make the ocm-provider endpoint in the ocmd service unprotected

   https://github.com/cs3org/reva/issues/1751
   https://github.com/cs3org/reva/pull/1831

 * Bugfix #1808: Use empty array in OCS Notifications endpoints

   https://github.com/cs3org/reva/pull/1808

 * Bugfix #1825: Raise max grpc message size

   As a workaround for listing larger folder we raised the `MaxCallRecvMsgSize` to 10MB. This
   should be enough for ~15k files. The proper fix is implementing ListContainerStream in the
   gateway, but we needed a way to test the web ui with larger collections.

   https://github.com/cs3org/reva/pull/1825

 * Bugfix #1828: Send a proper XML header with error messages

   https://github.com/cs3org/reva/pull/1828

 * Change #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7

   In order to upgrade mattn/go-sqlite3 to v1.14.7, the odic provider service is removed, which
   is possible because it is not used anymore

   https://github.com/cs3org/reva/pull/1828
   https://github.com/owncloud/ocis/pull/2209

 * Enhancement #1834: Add API key to Mentix GOCDB connector

   The PI (programmatic interface) of the GOCDB will soon require an API key; this PR adds the
   ability to configure this key in Mentix.

   https://github.com/cs3org/reva/pull/1834

 * Enhancement #1855: Minor optimization in parsing EOS ACLs

   https://github.com/cs3org/reva/pull/1855

 * Enhancement #1873: Update the EOS image tag to be for revad-eos image

   https://github.com/cs3org/reva/pull/1873

 * Enhancement #1802: Introduce list spaces

   The ListStorageSpaces call now allows listing all user homes and shared resources using a
   storage space id. The gateway will forward requests to a specific storage provider when a
   filter by id is given. Otherwise it will query all storage providers. Results will be
   deduplicated. Currently, only the decomposed fs storage driver implements the necessary
   logic to demonstrate the implmentation. A new `/dav/spaces` WebDAV endpoint to directly
   access a storage space is introduced in a separate PR.

   https://github.com/cs3org/reva/pull/1802
   https://github.com/cs3org/reva/pull/1803

 * Enhancement #1849: Add readonly interceptor

   The readonly interceptor could be used to configure a storageprovider in readonly mode. This
   could be handy in some migration scenarios.

   https://github.com/cs3org/reva/pull/1849

 * Enhancement #1875: Simplify resource comparison

   We replaced ResourceEqual with ResourceIDEqual where possible.

   https://github.com/cs3org/reva/pull/1875

 * Enhancement #1827: Support trashbin sub paths in the recycle API

   The recycle API could only act on the root items of the trashbin. Meaning if you delete a deep
   tree, you couldn't restore just one file from that tree but you had to restore the whole tree. Now
   listing, restoring and purging work also for sub paths in the trashbin.

   https://github.com/cs3org/reva/pull/1827


