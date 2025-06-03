
---
title: "v3.0.0"
linkTitle: "v3.0.0"
weight: 40
description: >
  Changelog for Reva v3.0.0 (2025-06-03)
---

Changelog for reva 3.0.0 (2025-06-03)
=======================================

The following sections list the changes in reva 3.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5082: Wrong quota total reported
 * Fix #5120: Allow folders to be un-favorited
 * Fix #5123: Use binary client for Attrs
 * Fix #5119: Stop sending non-UTF8 strings over gRPC
 * Fix #5124: Apps: fixed UserAgent matching
 * Fix #5122: Use the correct eos app header
 * Fix #5143: Eosgrpc: fixed panic with ACLs handling
 * Fix #5156: Handlenew failed to handle spaces id
 * Fix #5150: GetSpace with spaceID from URL
 * Fix #5153: Ocdav/put response compatible with spaces
 * Fix #5133: Fix broken handling of range requests
 * Fix #5149: ListMyOfficeFiles
 * Fix #5148: ListMyOfficeFiles
 * Fix #5064: Impersonate owner on ListRevisions
 * Fix #5166: Check for spaces in listshares
 * Fix #5084: Use logicalbytes instead of bytes
 * Fix #5044: Return an error when EOS List errors
 * Fix #5072: Impersonate owner on Revisions
 * Fix #5151: Spaces + apps broken
 * Fix #5142: Renamed mock APIs
 * Fix #5152: Revisions in spaces
 * Chg #5155: ListMyOfficeFiles only lists projects, not home
 * Chg #5105: Move publicshare sql driver
 * Enh #5113: Log simplified user agent in apps
 * Enh #5107: Add file extension in the returning app URL log message
 * Enh #5085: Extend app /notify endpoint to allow reporting errors
 * Enh #5110: Log acl payload on AddACL on EOS over gRPC
 * Enh #5186: Clean up EOS driver
 * Enh #5132: Add expiration dates reinforcement
 * Enh #5158: Add expire date to capabilities
 * Enh #5080: Log viewmode in the returning app URL message
 * Enh #5160: Upgrade to go1.24
 * Enh #4940: Refactoring of /home
 * Enh #5174: Upgrade jwt library
 * Enh #5134: Libregraph API expansion
 * Enh #5127: ListMyOfficeFiles
 * Enh #5129: Add makefile option for local plugins
 * Enh #5076: Implement OCM 1.2
 * Enh #5029: Pseudo-transactionalize sharing
 * Enh #5109: Extra logging in publicshares
 * Enh #5167: Improved logging of rhttp router
 * Enh #4404: Add support for Spaces

Details
-------

 * Bugfix #5082: Wrong quota total reported

   The EOS `QuotaInfo` struct had fields for `AvailableBytes` and `AvailableInodes`, but these
   were used to mean the total. This is fixed now.

   https://github.com/cs3org/reva/pull/5082

 * Bugfix #5120: Allow folders to be un-favorited

   Currently, removing a folder from your favorites is broken, if you are the only one who has
   favorited it. This is because the UnsetAttr call to EOS over gRPC is failing. As a temporary
   workaround, we now always set it to empty.

   https://github.com/cs3org/reva/pull/5120

 * Bugfix #5123: Use binary client for Attrs

   EOS < 5.3 has a couple of bugs related to attributes: * Attributes can only be removed as root or
   the owner, but over gRPC we cannot become root * The recursive property is ignored on set
   attributes

   For these two issues, we circumvent them by calling the binary client until we have deployed EOS
   5.3

   https://github.com/cs3org/reva/pull/5123

 * Bugfix #5119: Stop sending non-UTF8 strings over gRPC

   EOS supports having non-UTF8 attributes, which get returned in a Stat. This is problematic for
   us, as we pass these attributes in the ArbitraryMetadata, which gets sent over gRPC. However,
   the protobuf language specification states:

   > A string must always contain UTF-8 encoded or 7-bit ASCII text, and cannot be longer than 2^32.

   An example of such an attribute is:

   
   User.$KERNEL.PURGE.SEC.FILEHASH="S��ϫ]���z��#1}��uU�v��8�L0R�9j�j��e?�2K�T<sJ�*�l���Dǭ��_[�>η�...��w�w[��Yg"

   We fix this by stripping non-UTF8 metadata entries before sending the ResourceInfo over gRPC

   https://github.com/cs3org/reva/pull/5119

 * Bugfix #5124: Apps: fixed UserAgent matching

   https://github.com/cs3org/reva/pull/5124

 * Bugfix #5122: Use the correct eos app header

   https://github.com/cs3org/reva/pull/5122

 * Bugfix #5143: Eosgrpc: fixed panic with ACLs handling

   Fixes a panic that happens when listing a folder where files have no SysACLs, parent == nil and
   versionFolder has ACLs.

   https://github.com/cs3org/reva/pull/5143

 * Bugfix #5156: Handlenew failed to handle spaces id

   HandleNew, which creates new office files etc., tried to parse the parent container ref via
   spaces, and had no fallback for non-spaces. This is now fixed.

   https://github.com/cs3org/reva/pull/5156

 * Bugfix #5150: GetSpace with spaceID from URL

   `getSpace` was getting the first element of path as the ID. This fix adds a new function
   `GetIdFromPath`, which uses the last element of the path as the ID.

   https://github.com/cs3org/reva/pull/5150

 * Bugfix #5153: Ocdav/put response compatible with spaces

   Checks if `spaces` is enabled before returning the `fileid` after a PUT request.

   https://github.com/cs3org/reva/pull/5153

 * Bugfix #5133: Fix broken handling of range requests

   Currently, the video preview in public links is broken, because the browser sends a "Range:
   bytes=0-" request. Since EOS over gRPC returns a ReadCloser on the file, which is not seekable,
   Reva currently returns a 416 RequestedRangeNotSatisfiable response, breaking the video
   preview.

   This PR modifies this behaviour to ignore the Range request in such cases.

   Additionally, some errors where removed. For example, when the request does not contain
   bytes=, Reva currently returns an error. However, RFC 7233 states:

   > An origin server MUST ignore a Range header field that contains a range unit it does not
   understand

   Thus, we now ignore these requests instead of returning a 416.

   https://github.com/cs3org/reva/pull/5133

 * Bugfix #5149: ListMyOfficeFiles

   Regex was simplified, and a cache was created to keep version folder fileinfo, to make sure we
   don't need a stat for every result

   https://github.com/cs3org/reva/pull/5149

 * Bugfix #5148: ListMyOfficeFiles

   There was a bug in the regex for excel files: "(.*?)(.xls|.XLS|)[x|X]?$" contains an extra "|"

   https://github.com/cs3org/reva/pull/5148

 * Bugfix #5064: Impersonate owner on ListRevisions

   ListRevisions is currently broken for projects, because this happens on behalf of the user,
   instead of the owner of the file. This behaviour is changed to do the call on behalf of the owner
   (if we are in a non-home space).

   https://github.com/cs3org/reva/pull/5064

 * Bugfix #5166: Check for spaces in listshares

   Opening files in their app from the "Shared with me" view was broken, because we returned spaces
   ids even on old clients.

   https://github.com/cs3org/reva/pull/5166

 * Bugfix #5084: Use logicalbytes instead of bytes

   EOS gRPC used `usedbytes` instead of `usedlogicalbytes` for calculating quota, resulting in
   a wrong view

   https://github.com/cs3org/reva/pull/5084

 * Bugfix #5044: Return an error when EOS List errors

   If we get an error while reading items, we now return the error to the user and break off the List
   operation We do not want to return a partial list, because then a sync client may delete local
   files that are missing on the server

   https://github.com/cs3org/reva/pull/5044

 * Bugfix #5072: Impersonate owner on Revisions

   The current implementation of Download / Restore Revisions is not consistent with
   ListRevisions, where we impersonate the owner in projects. We now also impersonate in the case
   of Download and Restore.

   https://github.com/cs3org/reva/pull/5072

 * Bugfix #5151: Spaces + apps broken

   There were still some places where Reva assumed that we are running spaces; while not verifying
   this. This caused non-spaces WebUIs to break. This is fixed now.

   https://github.com/cs3org/reva/pull/5151

 * Bugfix #5142: Renamed mock APIs

   The once called "mock" space APIs have been renamed as they provide static content to the web
   frontend

   https://github.com/cs3org/reva/pull/5142

 * Bugfix #5152: Revisions in spaces

   The PR that merged spaces compatibility broke listing revisions. This is now fixed.

   https://github.com/cs3org/reva/pull/5152

 * Change #5155: ListMyOfficeFiles only lists projects, not home

   ListMyOfficeFiles now shows only one project or the user's home, not both at the same time

   https://github.com/cs3org/reva/pull/5155

 * Change #5105: Move publicshare sql driver

   The publicshare sql driver has been moved to reva-plugins, to be consistent with the usershare
   sql driver.

   https://github.com/cs3org/reva/pull/5105

 * Enhancement #5113: Log simplified user agent in apps

   https://github.com/cs3org/reva/pull/5113

 * Enhancement #5107: Add file extension in the returning app URL log message

   https://github.com/cs3org/reva/pull/5107

 * Enhancement #5085: Extend app /notify endpoint to allow reporting errors

   https://github.com/cs3org/reva/pull/5085

 * Enhancement #5110: Log acl payload on AddACL on EOS over gRPC

   https://github.com/cs3org/reva/pull/5110

 * Enhancement #5186: Clean up EOS driver

   * removed unused eos drivers (home, grpc, grpchome) * removed dependency on wrapper

   https://github.com/cs3org/reva/pull/5186

 * Enhancement #5132: Add expiration dates reinforcement

   Public links of folders with RW permissions now have a configurable expiration date.

   https://github.com/cs3org/reva/pull/5132

 * Enhancement #5158: Add expire date to capabilities

   https://github.com/cs3org/reva/pull/5158

 * Enhancement #5080: Log viewmode in the returning app URL message

   https://github.com/cs3org/reva/pull/5080

 * Enhancement #5160: Upgrade to go1.24

   Upgrade to go1.24

   https://github.com/cs3org/reva/pull/5160

 * Enhancement #4940: Refactoring of /home

   https://github.com/cs3org/reva/pull/4940

 * Enhancement #5174: Upgrade jwt library

   Update the golang-jwt library to v5

   https://github.com/cs3org/reva/pull/5174

 * Enhancement #5134: Libregraph API expansion

   Several new libregraph API endpoints have been added in ocgraph. These endpoints are used by
   the updated front-end. Concretely, endpoints have been added for * searching users *
   searching groups * creating shares * creating public links

   https://github.com/cs3org/reva/pull/5134

 * Enhancement #5127: ListMyOfficeFiles

   This PR implements the temporary ListMyOfficeFiles functionality

   https://github.com/cs3org/reva/pull/5127

 * Enhancement #5129: Add makefile option for local plugins

   Added a new Make target that takes a local copy of reva-plugins

   https://github.com/cs3org/reva/pull/5129

 * Enhancement #5076: Implement OCM 1.2

   This PR brings in the implementation of parts of OpenCloudMesh 1.2, including: * Adopting the
   new properties of the OCM 1.2 payloads, without implementing any new functionality for now. In
   particular, any non-empty `requirement` in a share will be rejected (a test was added for
   that). * Extending the OCM discovery endpoint. * Using the remote OCM discovery endpoint to
   establish the full URL of an incoming remote share, regardless if provided or not. When sending
   a share, though, we still send a full URL. * Caching the webdav client used to connect to remote
   endpoints, with added compatibility to OCM 1.0 remote servers. * Some refactoring and
   consolidation of duplicated code. * Improved logging.

   https://github.com/cs3org/reva/pull/5076

 * Enhancement #5029: Pseudo-transactionalize sharing

   Currently, sharing is not transactionalized: setting ACLs and writing the share to the db is
   completely independent. In the current situation, shares are written to the db before setting
   the ACL, letting users falsely believe that they successfully shared a resource, even if
   setting the ACL afterwards fails. his enhancement improves the situation by doing the least
   reliable (setting ACLs on EOS) first: a) first pinging the db b) writing the ACLs c) writing to
   the db

   https://github.com/cs3org/reva/pull/5029

 * Enhancement #5109: Extra logging in publicshares

   Some extra log lines in the public-files DAV handler have been added

   https://github.com/cs3org/reva/pull/5109

 * Enhancement #5167: Improved logging of rhttp router

   https://github.com/cs3org/reva/pull/5167

 * Enhancement #4404: Add support for Spaces

   Credits to @gmgigi96

   https://github.com/cs3org/reva/pull/4404


