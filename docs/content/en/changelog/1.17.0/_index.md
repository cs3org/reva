
---
title: "v1.17.0"
linkTitle: "v1.17.0"
weight: 40
description: >
  Changelog for Reva v1.17.0 (2021-12-09)
---

Changelog for reva 1.17.0 (2021-12-09)
=======================================

The following sections list the changes in reva 1.17.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2305: Make sure /app/new takes `target` as absolute path
 * Fix #2303: Fix content disposition header for public links files
 * Fix #2316: Fix the share types in propfinds
 * Fix #2803: Fix app provider for editor public links
 * Fix #2298: Remove share refs from trashbin
 * Fix #2309: Remove early finish for zero byte file uploads
 * Fix #1941: Fix TUS uploads with transfer token only
 * Chg #2210: Fix app provider new file creation and improved error codes
 * Enh #2217: OIDC auth driver for ESCAPE IAM
 * Enh #2256: Return user type in the response of the ocs GET user call
 * Enh #2315: Add new attributes to public link propfinds
 * Enh #2740: Implement space membership endpoints
 * Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
 * Enh #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

Details
-------

 * Bugfix #2305: Make sure /app/new takes `target` as absolute path

   A mini-PR to make the `target` parameter absolute (by prepending `/` if missing).

   https://github.com/cs3org/reva/pull/2305

 * Bugfix #2303: Fix content disposition header for public links files

   https://github.com/cs3org/reva/pull/2303
   https://github.com/cs3org/reva/pull/2297
   https://github.com/cs3org/reva/pull/2332
   https://github.com/cs3org/reva/pull/2346

 * Bugfix #2316: Fix the share types in propfinds

   The share types for public links were not correctly added to propfinds.

   https://github.com/cs3org/reva/pull/2316

 * Bugfix #2803: Fix app provider for editor public links

   Fixed opening the app provider in public links with the editor permission. The app provider
   failed to open the file in read write mode.

   https://github.com/owncloud/ocis/issues/2803
   https://github.com/cs3org/reva/pull/2310

 * Bugfix #2298: Remove share refs from trashbin

   https://github.com/cs3org/reva/pull/2298

 * Bugfix #2309: Remove early finish for zero byte file uploads

   We've fixed the upload of zero byte files by removing the early upload finishing mechanism.

   https://github.com/cs3org/reva/issues/2309
   https://github.com/owncloud/ocis/issues/2609

 * Bugfix #1941: Fix TUS uploads with transfer token only

   TUS uploads had been stopped when the user JWT token expired, even if only the transfer token
   should be validated. Now uploads will continue as intended.

   https://github.com/cs3org/reva/pull/1941

 * Change #2210: Fix app provider new file creation and improved error codes

   We've fixed the behavior for the app provider when creating new files. Previously the app
   provider would overwrite already existing files when creating a new file, this is now handled
   and prevented. The new file endpoint accepted a path to a file, but this does not work for spaces.
   Therefore we now use the resource id of the folder where the file should be created and a filename
   to create the new file. Also the app provider returns more useful error codes in a lot of cases.

   https://github.com/cs3org/reva/pull/2210

 * Enhancement #2217: OIDC auth driver for ESCAPE IAM

   This enhancement allows for oidc token authentication via the ESCAPE IAM service.
   Authentication relies on mappings of ESCAPE IAM groups to REVA users. For a valid token, if at
   the most one group from the groups claim is mapped to one REVA user, authentication can take
   place.

   https://github.com/cs3org/reva/pull/2217

 * Enhancement #2256: Return user type in the response of the ocs GET user call

   https://github.com/cs3org/reva/pull/2256

 * Enhancement #2315: Add new attributes to public link propfinds

   Added a new property "oc:signature-auth" to public link propfinds. This is a necessary change
   to be able to support archive downloads in password protected public links.

   https://github.com/cs3org/reva/pull/2315

 * Enhancement #2740: Implement space membership endpoints

   Implemented endpoints to add and remove members to spaces.

   https://github.com/owncloud/ocis/issues/2740
   https://github.com/cs3org/reva/pull/2250

 * Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

 * Enhancement #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

   Some IDPs don't support the "preferred_username" claim. Fallback to the "email" claim in that
   case.

   https://github.com/cs3org/reva/pull/2314


