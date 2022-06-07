
---
title: "v2.5.0"
linkTitle: "v2.5.0"
weight: 40
description: >
  Changelog for Reva v2.5.0 (2022-06-07)
---

Changelog for reva 2.5.0 (2022-06-07)
=======================================

The following sections list the changes in reva 2.5.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2909: The decomposedfs now checks the GetPath permission
 * Fix #2899: Empty meta requests should return body
 * Fix #2928: Fix mkcol response code
 * Fix #2907: Correct share jail child aggregation
 * Fix #3810: Fix unlimitted quota in spaces
 * Fix #3498: Check user permissions before updating/removing public shares
 * Fix #2904: Share jail now works properly when accessed as a space
 * Fix #2903: User owncloudsql now uses the correct userid
 * Chg #2920: Clean up the propfind code
 * Chg #2913: Rename ocs parameter "space_ref"
 * Enh #2919: EOS Spaces implementation
 * Enh #2888: Introduce spaces field mask
 * Enh #2922: Refactor webdav error handling

Details
-------

 * Bugfix #2909: The decomposedfs now checks the GetPath permission

   After fixing the meta endpoint and introducing the fieldmask the GetPath call is made directly
   to the storageprovider. The decomposedfs now checks if the current user actually has the
   permission to get the path. Before the two previous PRs this was covered by the list storage
   spaces call which used a stat request and the stat permission.

   https://github.com/cs3org/reva/pull/2909

 * Bugfix #2899: Empty meta requests should return body

   Meta requests with no resourceID should return a multistatus response body with a 404 part.

   https://github.com/cs3org/reva/pull/2899

 * Bugfix #2928: Fix mkcol response code

   We now return the correct response code when an mkcol fails.

   https://github.com/cs3org/reva/pull/2928

 * Bugfix #2907: Correct share jail child aggregation

   We now add up the size of all mount points when aggregating the size for a child with the same name.
   Furthermore, the listing should no longer contain duplicate entries.

   https://github.com/cs3org/reva/pull/2907

 * Bugfix #3810: Fix unlimitted quota in spaces

   Fixed the quota check when unlimitting a space, i.e. when setting the quota to "0".

   https://github.com/owncloud/ocis/issues/3810
   https://github.com/cs3org/reva/pull/2895

 * Bugfix #3498: Check user permissions before updating/removing public shares

   Added permission checks before updating or deleting public shares. These methods previously
   didn't enforce the users permissions.

   https://github.com/owncloud/ocis/issues/3498
   https://github.com/cs3org/reva/pull/3900

 * Bugfix #2904: Share jail now works properly when accessed as a space

   When accessing shares via the virtual share jail we now build correct relative references
   before forwarding the requests to the correct storage provider.

   https://github.com/cs3org/reva/pull/2904

 * Bugfix #2903: User owncloudsql now uses the correct userid

   https://github.com/cs3org/reva/pull/2903

 * Change #2920: Clean up the propfind code

   Cleaned up the ocdav propfind code to make it more readable.

   https://github.com/cs3org/reva/pull/2920

 * Change #2913: Rename ocs parameter "space_ref"

   We decided to deprecate the parameter "space_ref". We decided to use "space" parameter
   instead. The difference is that "space" must not contain a "path". The "path" parameter can be
   used in combination with "space" to create a relative path request

   https://github.com/cs3org/reva/pull/2913

 * Enhancement #2919: EOS Spaces implementation

   https://github.com/cs3org/reva/pull/2919

 * Enhancement #2888: Introduce spaces field mask

   We now use a field mask to select which properties to retrieve when looking up storage spaces.
   This allows the gateway to only ask for `root` when trying to forward id or path based requests.

   https://github.com/cs3org/reva/pull/2888

 * Enhancement #2922: Refactor webdav error handling

   We made more webdav handlers return a status code and error to unify error rendering

   https://github.com/cs3org/reva/pull/2922


