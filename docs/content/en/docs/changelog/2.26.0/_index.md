
---
title: "v2.26.0"
linkTitle: "v2.26.0"
weight: 40
description: >
  Changelog for Reva v2.26.0 (2024-10-21)
---

Changelog for reva 2.26.0 (2024-10-21)
=======================================

The following sections list the changes in reva 2.26.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4880: Kept historical resource naming in activity
*   Fix #4874: Fix rename activity
*   Fix #4881: Log levels
*   Fix #4884: Fix OCM upload crush
*   Fix #4872: Return 409 conflict when a file was already created
*   Fix #4887: Fix ShareCache concurrency panic
*   Fix #4876: Fix share jail mountpoint parent id
*   Fix #4879: Fix trash-bin propfind panic
*   Fix #4888: Fix upload session bugs
*   Fix #4560: Always select next before making CS3 calls for propfinds
*   Enh #4893: Bump dependencies and go to 1.22.8
*   Enh #4890: Bump golangci-lint to 1.61.0
*   Enh #4886: Add new Mimetype ggp
*   Enh #4809: Implement OCM well-known endpoint
*   Enh #4889: Improve posixfs stability and performance
*   Enh #4882: Indicate template conversion capabilties on apps

Details
-------

*   Bugfix #4880: Kept historical resource naming in activity

   Kept historical resource naming after renaming in activity for shares and public links.

   https://github.com/owncloud/ocis/issues/10210
   https://github.com/cs3org/reva/pull/4880

*   Bugfix #4874: Fix rename activity

   We fixed the activity when file with file-id gives move activity instead of rename.

   https://github.com/owncloud/ocis/issues/9744
   https://github.com/cs3org/reva/pull/4874

*   Bugfix #4881: Log levels

   We changed the following log levels:

   - `ERROR` to `DEBUG` in `internal/grpc/services/usershareprovider` when getting received
   shares

   https://github.com/cs3org/reva/pull/4881

*   Bugfix #4884: Fix OCM upload crush

   We fixed an issue where a federated instance crashed when uploading a file to a remote folder.
   Fixed the cleanup blob and meta of the uploaded files.

   https://github.com/cs3org/reva/pull/4884

*   Bugfix #4872: Return 409 conflict when a file was already created

   We now return the correct 409 conflict status code when a file was already created by another
   upload.

   https://github.com/cs3org/reva/pull/4872

*   Bugfix #4887: Fix ShareCache concurrency panic

   We fixed an issue where concurrently read and write operations led to a panic in the ShareCache.

   https://github.com/cs3org/reva/pull/4887

*   Bugfix #4876: Fix share jail mountpoint parent id

   Stating a share jail mountpoint now returns the share jail root as the parent id.

   https://github.com/owncloud/ocis/issues/9933
   https://github.com/cs3org/reva/pull/4876

*   Bugfix #4879: Fix trash-bin propfind panic

   We fixed an issue where a trash-bin `propfind` request panicked due to a failed and therefore
   `nil` resource reference lookup.

   https://github.com/cs3org/reva/pull/4879

*   Bugfix #4888: Fix upload session bugs

   We fixed an issue that caused a panic when we could not open a file to calculate checksums.
   Furthermore, we now delete the upload session .lock file on cleanup.

   https://github.com/cs3org/reva/pull/4888

*   Bugfix #4560: Always select next before making CS3 calls for propfinds

   We now select the next client more often to spread out load

   https://github.com/cs3org/reva/pull/4560

*   Enhancement #4893: Bump dependencies and go to 1.22.8

   https://github.com/cs3org/reva/pull/4893

*   Enhancement #4890: Bump golangci-lint to 1.61.0

   https://github.com/cs3org/reva/pull/4890

*   Enhancement #4886: Add new Mimetype ggp

   Adds a new mimetype application/vnd.geogebra.pinboard (ggp) to the app-registry

   https://github.com/cs3org/reva/pull/4886

*   Enhancement #4809: Implement OCM well-known endpoint

   The `wellknown` service now implements the `/.well-known/ocm` endpoint for OCM discovery.
   The unused endpoints for openid connect and webfinger have been removed. This aligns the
   wellknown implementation with the master branch.

   https://github.com/cs3org/reva/pull/4809

*   Enhancement #4889: Improve posixfs stability and performance

   The posixfs storage driver saw a number of bugfixes and optimizations.

   https://github.com/cs3org/reva/pull/4889
   https://github.com/cs3org/reva/pull/4877

*   Enhancement #4882: Indicate template conversion capabilties on apps

   We added information to the available app providers to indicate which mimetypes can be used for
   template conversion.

   https://github.com/cs3org/reva/pull/4882

