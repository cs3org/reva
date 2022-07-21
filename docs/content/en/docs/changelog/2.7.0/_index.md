
---
title: "v2.7.0"
linkTitle: "v2.7.0"
weight: 40
description: >
  Changelog for Reva v2.7.0 (2022-07-15)
---

Changelog for reva 2.7.0 (2022-07-15)
=======================================

The following sections list the changes in reva 2.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3075: Check permissions of the move operation destination
 * Fix #3036: Fix revad with EOS docker image
 * Fix #3037: Add uid- and gidNumber to LDAP queries
 * Fix #4061: Forbid resharing with higher permissions
 * Fix #3017: Removed unused gateway config "commit_share_to_storage_ref"
 * Fix #3031: Return proper response code when detecting recursive copy/move operations
 * Fix #3071: Make CS3 sharing drivers parse legacy resource id
 * Fix #3035: Prevent cross space move
 * Fix #3074: Send storage provider and space id to wopi server
 * Fix #3022: Improve the sharing internals
 * Fix #2977: Test valid filename on spaces tus upload
 * Chg #3006: Use spaceID on the cs3api
 * Enh #3043: Introduce LookupCtx for index interface
 * Enh #3009: Prevent recursive copy/move operations
 * Enh #2977: Skip space lookup on space propfind

Details
-------

 * Bugfix #3075: Check permissions of the move operation destination

   We now properly check the permissions on the target of move operations.

   https://github.com/owncloud/ocis/issues/4192
   https://github.com/cs3org/reva/pull/3075

 * Bugfix #3036: Fix revad with EOS docker image

   We've fixed the revad with EOS docker image. Previously the revad binary was build on Alpine and
   not executable on the final RHEL based image.

   https://github.com/cs3org/reva/issues/3036

 * Bugfix #3037: Add uid- and gidNumber to LDAP queries

   For the EOS storage to work correctly the uid- and gidNumber attributes need to be populated.

   https://github.com/cs3org/reva/pull/3037

 * Bugfix #4061: Forbid resharing with higher permissions

   When creating a public link from a viewer share a user was able to set editor permissions on that
   link. This was because of a missing check that is added now

   https://github.com/owncloud/ocis/issues/4061
   https://github.com/owncloud/ocis/issues/3881
   https://github.com/owncloud/ocis/pull/4077

 * Bugfix #3017: Removed unused gateway config "commit_share_to_storage_ref"

   We've removed the unused gateway configuration option "commit_share_to_storage_ref".

   https://github.com/cs3org/reva/pull/3017

 * Bugfix #3031: Return proper response code when detecting recursive copy/move operations

   We changed the ocdav response code to "409 - Conflict" when a recursive operation was detected.

   https://github.com/cs3org/reva/pull/3031

 * Bugfix #3071: Make CS3 sharing drivers parse legacy resource id

   The CS3 public and user sharing drivers will now correct a resource id that is missing a spaceid
   when it can split the storageid.

   https://github.com/cs3org/reva/pull/3071

 * Bugfix #3035: Prevent cross space move

   Decomposedfs now prevents moving across space boundaries

   https://github.com/cs3org/reva/pull/3035

 * Bugfix #3074: Send storage provider and space id to wopi server

   We are now concatenating storage provider id and space id into the endpoint that is sent to the
   wopiserver

   https://github.com/cs3org/reva/issues/3074

 * Bugfix #3022: Improve the sharing internals

   We cleaned up the sharing code validation and comparisons.

   https://github.com/cs3org/reva/pull/3022

 * Bugfix #2977: Test valid filename on spaces tus upload

   Tus uploads in spaces now also test valid filenames.

   https://github.com/owncloud/ocis/issues/3050
   https://github.com/cs3org/reva/pull/2977

 * Change #3006: Use spaceID on the cs3api

   We introduced a new spaceID field on the cs3api to implement the spaces feature in a cleaner way.

   https://github.com/cs3org/reva/pull/3006

 * Enhancement #3043: Introduce LookupCtx for index interface

   The index interface now has a new LookupCtx that can look up multiple values so we can more
   efficiently look up multiple shares by id. It also takes a context so we can pass on the trace
   context to the CS3 backend

   https://github.com/cs3org/reva/pull/3043

 * Enhancement #3009: Prevent recursive copy/move operations

   We changed the ocs API to prevent copying or moving a folder into one of its children.

   https://github.com/cs3org/reva/pull/3009

 * Enhancement #2977: Skip space lookup on space propfind

   We now construct the space id from the /dav/spaces URL intead of making a request to the
   registry.

   https://github.com/owncloud/ocis/issues/1277
   https://github.com/owncloud/ocis/issues/2144
   https://github.com/owncloud/ocis/issues/3073
   https://github.com/cs3org/reva/pull/2977


