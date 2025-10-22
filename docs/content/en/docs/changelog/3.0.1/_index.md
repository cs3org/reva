
---
title: "v3.0.1"
linkTitle: "v3.0.1"
weight: 999699
description: >
  Changelog for Reva v3.0.1 (2025-07-04)
---

Changelog for reva 3.0.1 (2025-07-04)
=======================================

The following sections list the changes in reva 3.0.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5213: Home creation
 * Fix #5190: List file versions
 * Fix #5189: Shares parent reference
 * Fix #5204: Restore trashbin
 * Fix #5219: Reva v3
 * Fix #5198: Let sharees list versions
 * Fix #5216: Download / restore versions in spaces
 * Enh #5220: Clean up obosolete OCIS tests
 * Enh #4864: Add HTTP header to disable versioning on EOS
 * Enh #5201: EOS gRPC cleanup
 * Enh #4883: Use newfind command in EOS
 * Enh #5211: Add error code to DAV responses
 * Enh #5210: Libregraph permission support
 * Enh #5205: Ignore unknown routes
 * Enh #5197: Pprof improvements
 * Enh #5217: Spaces improvements

Details
-------

 * Bugfix #5213: Home creation

   https://github.com/cs3org/reva/pull/5213

 * Bugfix #5190: List file versions

   - moved versions-related functions to utils package - new `spaceHref` function for listing
   file versions - adapts code from #2855 for restoring and downloading file versions - add parent
   info to propfind response - add space info to parent reference

   https://github.com/cs3org/reva/pull/5190

 * Bugfix #5189: Shares parent reference

   - change: replace `md.Id.SpaceID` with `<storage-id>?<space-id>` - fix: parentReference -
   add space info to id - removes double encoding of driveId - new function to return relative path
   inside a space root - refactor space utils: - reorder functions (Encode > Decode > Parse) -
   returns `SpaceID` instead of `path` in `DecodeResourceID` - new comments

   https://github.com/cs3org/reva/pull/5189

 * Bugfix #5204: Restore trashbin

   https://github.com/cs3org/reva/pull/5204

 * Bugfix #5219: Reva v3

   Made reva module v3, to align with the github release

   https://github.com/cs3org/reva/pull/5219

 * Bugfix #5198: Let sharees list versions

   https://github.com/cs3org/reva/pull/5198/

 * Bugfix #5216: Download / restore versions in spaces

   * Some extra logging * Fixed a bug in IsVersionFolder * Fixed a bug in handling GET requests on the
   VersionsHandler

   https://github.com/cs3org/reva/pull/5216

 * Enhancement #5220: Clean up obosolete OCIS tests

   https://github.com/cs3org/reva/pull/5220

 * Enhancement #4864: Add HTTP header to disable versioning on EOS

   https://github.com/cs3org/reva/pull/4864
   This
   enhancement
   introduces
   a
   new
   header,
   %60X-Disable-Versioning%60,
   on
   PUT
   requests.
   EOS
   will
   not
   version
   this
   file
   save
   whenever
   this
   header
   is
   set
   with
   a
   truthy
   value.
   See
   also:

 * Enhancement #5201: EOS gRPC cleanup

   Remove reliance on binary client for some operations, split up EOS gRPC driver into several
   files

   https://github.com/cs3org/reva/pull/5201

 * Enhancement #4883: Use newfind command in EOS

   The EOS binary storage driver was still using EOS's oldfind command, which is deprecated. We
   now moved to the new find command, for which an extra flag (--skip-version-dirs) is needed.

   https://github.com/cs3org/reva/pull/4883

 * Enhancement #5211: Add error code to DAV responses

   - code adpated from the edge branch (#4749 and #4653) - new `errorCode` parameter in `Marshal`
   function

   https://github.com/cs3org/reva/pull/5211

 * Enhancement #5210: Libregraph permission support

   Extension of the libregraph API to fix the following issues: * Creating links / shares now gets a
   proper response * Support for updating links / shares * Support for deleting links / shares *
   Removal of unsupported roles from /roleDefinitions endpoint

   https://github.com/cs3org/reva/pull/5210

 * Enhancement #5205: Ignore unknown routes

   Currently, the gateway crashes with a fatal error if it encounters any unknown routes in the
   routing table. Instead, we log the error and ignore the routes, which should make upgrades in
   the routing table easier.

   https://github.com/cs3org/reva/pull/5205

 * Enhancement #5197: Pprof improvements

   https://github.com/cs3org/reva/pull/5197

 * Enhancement #5217: Spaces improvements

   Extended libregraph API, fixed restoring / downloading revisions in spaces

   https://github.com/cs3org/reva/pull/5217


