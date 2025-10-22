
---
title: "v3.2.1"
linkTitle: "v3.2.1"
weight: 999679
description: >
  Changelog for Reva v3.2.1 (2025-10-13)
---

Changelog for reva 3.2.1 (2025-10-13)
=======================================

The following sections list the changes in reva 3.2.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5340: Support requests to /app/new on space root without container ID
 * Fix #5346: Disallow setting link expiry in the past
 * Fix #5284: Make COPY work in public links in spaces
 * Fix #5130: Eos: fixed use of "app" tag
 * Fix #5347: Make eosmedia work with new space structure
 * Fix #5335: Fix EOS tokens + EOS 5.3.22
 * Fix #5349: Fix filtering spaces by ID
 * Fix #5288: No home creation for machine auth
 * Fix #5333: Add user filter to OCS listSharesWithOthers
 * Fix #5314: Skip missing routes in dynamic router
 * Fix #5348: Fix nilpointers seen in prod
 * Fix #5294: Ignore errors on decorating projects
 * Fix #5345: Fix 502 for LW accs when OCM is enabled
 * Fix #5328: Handle OCM disabled in getPermissionsByCs3Reference
 * Fix #5342: ListMyOfficeFiles in spaces
 * Fix #5324: Register share sql driver
 * Fix #5337: Fix permission check for MOVE for LW accs
 * Fix #5336: SharedByMe works with deleted accounts
 * Fix #5326: SharedWithMe without OCM
 * Fix #5290: Fix nilpointers in spaces shares
 * Fix #5344: GetSpace did not work for homes
 * Fix #5361: Slash missing in public links
 * Fix #5351: Rollback OCM share creation if remote request fails
 * Fix #5350: Searching for federated users
 * Enh #5273: Add new storage driver cephmount
 * Enh #5332: Expose the ProjectsManager
 * Enh #5331: Add support for thumbnails in Public spaces
 * Enh #5287: Update multiple fields of a public link in the graphAPI
 * Enh #5291: Add support for filter on FindUsers
 * Enh #5292: Add support for changing language
 * Enh #5286: Add OCM CreateShare to LibreGraph implementation
 * Enh #5317: use GORM to manage preferences table
 * Enh #5289: Logging improvements
 * Enh #5293: Make preferences work for LW accounts
 * Enh #5339: NotifyUploads on dropzones in libregraph
 * Enh #5329: Implement ocgraph endpoint for hiding shares
 * Enh #5316: List OCM shares in LibreGraph implementation
 * Enh #5320: OCM Update, Delete and show OCM Shares in SharedWith view
 * Enh #5283: Public spaces
 * Enh #5319: Set Quota Improvements to consider new Project Quotas
 * Enh #5322: Import share sql driver
 * Enh #5334: Show public links to project admins
 * Enh #5301: Show shares to project admins

Details
-------

 * Bugfix #5340: Support requests to /app/new on space root without container ID

   For some reason, the web client was not sending the container ID when in a space root, so we also
   accept this (since we can deduce it from the Space ID)

   https://github.com/cs3org/reva/pull/5340

 * Bugfix #5346: Disallow setting link expiry in the past

   Aditionally, in some cases an earlier expiration date was accidentally overwritten by a later
   one. This is also now fixed.

   https://github.com/cs3org/reva/pull/5346

 * Bugfix #5284: Make COPY work in public links in spaces

   https://github.com/cs3org/reva/pull/5284/files

 * Bugfix #5130: Eos: fixed use of "app" tag

   Review of the eos.app query parameter/header to make it consistent and adapt to eos traffic
   tagging

   https://github.com/cs3org/reva/pull/5130

 * Bugfix #5347: Make eosmedia work with new space structure

   We have added a special case to the `spacesLevel` function to make eosmedia's spaces work. This
   is temporary, since we plan to get rid of this function and just use the SpaceID that comes from
   the projects database.

   https://github.com/cs3org/reva/pull/5347

 * Bugfix #5335: Fix EOS tokens + EOS 5.3.22

   Since EOS 5.3.22 does more strict checks on tokens, we: * Remove "x" bit from permissions in EOS
   token * assume role instead of using token when restoring / listing / downloading revisions

   https://github.com/cs3org/reva/pull/5335

 * Bugfix #5349: Fix filtering spaces by ID

   Rewrite handling of listing spaces with an ID filter. We now handle one or multiple filters
   properly, and the db driver supports querying both by SpaceID and StorageSpaceID

   https://github.com/cs3org/reva/pull/5349

 * Bugfix #5288: No home creation for machine auth

   When you sign in with auth type `machine`, no home should be created for the user

   https://github.com/cs3org/reva/pull/5288

 * Bugfix #5333: Add user filter to OCS listSharesWithOthers

   Fixes a memory leak where an OCS function would load all shares, because no filter for the
   current user was set

   https://github.com/cs3org/reva/pull/5333

 * Bugfix #5314: Skip missing routes in dynamic router

   Skip missing routes from logic.

   https://github.com/cs3org/reva/pull/5314
   https://github.com/cs3org/reva/pull/5318

 * Bugfix #5348: Fix nilpointers seen in prod

   * Fix log lines going to stderr, which polluted the logs * Fix possible nilpointer in
   getLinkUpdates * Fix possible nilpointer in eoshttp's `GETFile`

   https://github.com/cs3org/reva/pull/5348

 * Bugfix #5294: Ignore errors on decorating projects

   When decorating a project fails, we should ignore this project and still return a list of
   projects to the user

   https://github.com/cs3org/reva/pull/5294

 * Bugfix #5345: Fix 502 for LW accs when OCM is enabled

   When OCM is enabled, going to `/sharedWithMe` or `/sharedByMe` results in a 502. We now skip
   checking for OCM shares on LW accounts. Additionally, error handling in the libregraph layer
   has been improved

   https://github.com/cs3org/reva/pull/5345

 * Bugfix #5328: Handle OCM disabled in getPermissionsByCs3Reference

   When `OCMEnabled` is false, we should not query for OCM shares

   https://github.com/cs3org/reva/pull/5328

 * Bugfix #5342: ListMyOfficeFiles in spaces

   https://github.com/cs3org/reva/pull/5342

 * Bugfix #5324: Register share sql driver

   https://github.com/cs3org/reva/pull/5324

 * Bugfix #5337: Fix permission check for MOVE for LW accs

   The permission check for MOVE requests for lightweight accounts was broken: it checked
   whether the user has permission on the destination (which does not exist yet), instead of the
   destination's parent

   https://github.com/cs3org/reva/pull/5337

 * Bugfix #5336: SharedByMe works with deleted accounts

   The `/sharedByMe` endpoint now works, even if the user has shares with users that no longer
   exist

   https://github.com/cs3org/reva/pull/5336

 * Bugfix #5326: SharedWithMe without OCM

   If OCM is not enabled (i.e. the drivers are not there), then currently `sharedWithMe` and
   `sharedByMe` would fail. We now only log errors from OCM and still provide a response.

   https://github.com/cs3org/reva/pull/5326

 * Bugfix #5290: Fix nilpointers in spaces shares

   * Set `SkipFetchingGroupMembers` and `SkipFetchingUserGroups` to `true` in libregraph
   API; we don't need those there and fetching group members of `cern-all-users` crashes the
   daemon * Better nil handling * Move some conversion methods from `shares.go` to
   `conversions.go`

   https://github.com/cs3org/reva/pull/5290

 * Bugfix #5344: GetSpace did not work for homes

   The `getSpace` call did not work for homes. ListSpaces with filters only checked projects, not
   homes.

   https://github.com/cs3org/reva/pull/5344

 * Bugfix #5361: Slash missing in public links

   We used `path.Join` from the standard library for constructing URLs in two cases, which is
   wrong, as this will remove double slashes, leading to `https:/...`. This has now been fixed.

   https://github.com/cs3org/reva/pull/5361

 * Bugfix #5351: Rollback OCM share creation if remote request fails

   The OCM share request to the remote server happens after creating the share locally this means
   that if the remote request fails it should be rolled back (e.g. delete the share).

   https://github.com/cs3org/reva/pull/5351

 * Bugfix #5350: Searching for federated users

   Since the "sm:" prefix is no longer given by the frontend when searching for federated users
   this is removed and the if is replaced with a flag to perform the search only when OCM is enabled.

   https://github.com/cs3org/reva/pull/5350

 * Enhancement #5273: Add new storage driver cephmount

   The cephmount driver is now available.

   https://github.com/cs3org/reva/pull/5273

 * Enhancement #5332: Expose the ProjectsManager

   By making this component public instead of private, it can be used by cernboxcop

   https://github.com/cs3org/reva/pull/5332

 * Enhancement #5331: Add support for thumbnails in Public spaces

   To be configured with an extra thumbnail_path property in the public_space map.

   https://github.com/cs3org/reva/pull/5331

 * Enhancement #5287: Update multiple fields of a public link in the graphAPI

   - update is now able to handle multiple fields changes in the same request - new rules for
   expiration dates for public links with Editor permissions

   https://github.com/cs3org/reva/pull/5287

 * Enhancement #5291: Add support for filter on FindUsers

   The Graph API and the downstream CS3API for finding users now supports proper filtering on the
   user type

   https://github.com/cs3org/reva/pull/5291

 * Enhancement #5292: Add support for changing language

   The libregraph API now supports setting a `preferredLanguage` for a user

   https://github.com/cs3org/reva/pull/5292

 * Enhancement #5286: Add OCM CreateShare to LibreGraph implementation

   - LinkOrShare has been changed to include OCMShares and renamed - Web app now sends a POST
   request instead of a GET for OCM invite generation, this has been reflected in the backend. -
   Adds the IDP to the user id when returned if user is of type federated. - Cleaned up dead code since
   the /sciencemesh/create-share is no longer used. - Conversions now take OCM shares into
   account as well.

   https://github.com/cs3org/reva/pull/5286/

 * Enhancement #5317: use GORM to manage preferences table

   https://github.com/cs3org/reva/pull/5317

 * Enhancement #5289: Logging improvements

   * Improved logging in graphAPI and `GetQuota` * Moved one conversion function in graph api from
   `drives` to `conversions`

   https://github.com/cs3org/reva/pull/5289

 * Enhancement #5293: Make preferences work for LW accounts

   https://github.com/cs3org/reva/pull/5293

 * Enhancement #5339: NotifyUploads on dropzones in libregraph

   https://github.com/cs3org/reva/pull/5339

 * Enhancement #5329: Implement ocgraph endpoint for hiding shares

   https://github.com/cs3org/reva/pull/5329

 * Enhancement #5316: List OCM shares in LibreGraph implementation

   * Adapating the LibreGraph implementation to also list OCM shares * Bugfix for uploading (e.g.
   editing files) through OCM, headers had to be updated * Bugfix where stating a non existing file
   did not return a not found error - this made CreateDir and Touch fail since reva could not
   determine if the file exists. * Bugfix for creating files and directories through OCM,
   essentially the old logic was to try and stat the file to test different the different
   authentication methods basic and bearer, however this doesn't work for Touch and CreateDir
   since the stat is meant the fail here, the logic needs to be looked over. Further the OCM basic
   auth over webdav is broken and this causes these two operations to fail - since the stat fails and
   it reverts to basic auth which fails since it is currently broken.

   https://github.com/cs3org/reva/pull/5316

 * Enhancement #5320: OCM Update, Delete and show OCM Shares in SharedWith view

   * Adds support for displaying OCM shares in the SharedWith view when a file is selected. *
   Enables updating the permissions of those shares. * It also adds support for deleting OCM
   shares. * Some updateDrivePermissions and deleteDrivePermissions.

   https://github.com/cs3org/reva/pull/5320

 * Enhancement #5283: Public spaces

   We now support exposing other EOS areas as "public" spaces (with spaceType `SpaceTypePublic =
   "explorer"`)

   https://github.com/cs3org/reva/pull/5283

 * Enhancement #5319: Set Quota Improvements to consider new Project Quotas

   Unify SetQuota method signature across EOS client implementations

   https://github.com/cs3org/reva/pull/5319

 * Enhancement #5322: Import share sql driver

   Migrate share sql driver from github.com/cernbox/reva-plugins to github.com/cs3org/reva

   https://github.com/cs3org/reva/pull/5322

 * Enhancement #5334: Show public links to project admins

   https://github.com/cs3org/reva/pull/5334

 * Enhancement #5301: Show shares to project admins

   To realize this enhancement, the cache has also been modified to use Generics.

   https://github.com/cs3org/reva/pull/5301


