
---
title: "v1.15.0"
linkTitle: "v1.15.0"
weight: 40
description: >
  Changelog for Reva v1.15.0 (2021-10-26)
---

Changelog for reva 1.15.0 (2021-10-26)
=======================================

The following sections list the changes in reva 1.15.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2168: Override provider if was previously registered
 * Fix #2173: Fix archiver max size reached error
 * Fix #2167: Handle nil quota in decomposedfs
 * Fix #2153: Restrict EOS project spaces sharing permissions to admins and writers
 * Fix #2179: Fix the returned permissions for webdav uploads
 * Fix #2177: Retrieve the full path of a share when setting as
 * Chg #2479: Make apps able to work with public shares
 * Enh #2203: Add alerting webhook to SiteAcc service
 * Enh #2190: Update CODEOWNERS
 * Enh #2174: Inherit ACLs for files from parent directories
 * Enh #2152: Add a reference parameter to the getQuota request
 * Enh #2171: Add optional claim parameter to machine auth
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2135: Nextcloud test improvements
 * Enh #2180: Remove OCDAV options namespace parameter
 * Enh #2117: Add ocs cache warmup strategy for first request from the user
 * Enh #2170: Handle propfind requests for existing files
 * Enh #2165: Allow access to recycle bin for arbitrary paths outside homes
 * Enh #2193: Filter root paths according to user agent
 * Enh #2162: Implement the UpdateStorageSpace method
 * Enh #2189: Add user setting capability

Details
-------

 * Bugfix #2168: Override provider if was previously registered

   Previously if an AppProvider registered himself two times, for example after a failure, the
   mime types supported by the provider contained multiple times the same provider. Now this has
   been fixed, overriding the previous one.

   https://github.com/cs3org/reva/pull/2168

 * Bugfix #2173: Fix archiver max size reached error

   Previously in the total size count of the files being archived, the folders were taken into
   account, and this could cause a false max size reached error because the size of a directory is
   recursive-computed, causing the archive to be truncated. Now in the size count, the
   directories are skipped.

   https://github.com/cs3org/reva/pull/2173

 * Bugfix #2167: Handle nil quota in decomposedfs

   Do not nil pointer derefenrence when sending nil quota to decomposedfs

   https://github.com/cs3org/reva/issues/2167

 * Bugfix #2153: Restrict EOS project spaces sharing permissions to admins and writers

   https://github.com/cs3org/reva/pull/2153

 * Bugfix #2179: Fix the returned permissions for webdav uploads

   We've fixed the returned permissions for webdav uploads. It did not consider shares and public
   links for the permission calculation, but does so now.

   https://github.com/cs3org/reva/pull/2179
   https://github.com/cs3org/reva/pull/2151

 * Bugfix #2177: Retrieve the full path of a share when setting as

   Accepted or on shared by me

   https://github.com/cs3org/reva/pull/2177

 * Change #2479: Make apps able to work with public shares

   Public share receivers were not possible to use apps in public shares because the apps couldn't
   load the files in the public shares. This has now been made possible by changing the scope checks
   for public shares.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2143

 * Enhancement #2203: Add alerting webhook to SiteAcc service

   To integrate email alerting with the monitoring pipeline, a Prometheus webhook has been added
   to the SiteAcc service. Furthermore account settings have been extended/modified
   accordingly.

   https://github.com/cs3org/reva/pull/2203

 * Enhancement #2190: Update CODEOWNERS

   https://github.com/cs3org/reva/pull/2190

 * Enhancement #2174: Inherit ACLs for files from parent directories

   https://github.com/cs3org/reva/pull/2174

 * Enhancement #2152: Add a reference parameter to the getQuota request

   Implementation of [cs3org/cs3apis#147](https://github.com/cs3org/cs3apis/pull/147)

   Make the cs3apis accept a Reference in the getQuota Request to limit the call to a specific
   storage space.

   https://github.com/cs3org/reva/pull/2152
   https://github.com/cs3org/reva/pull/2178
   https://github.com/cs3org/reva/pull/2187

 * Enhancement #2171: Add optional claim parameter to machine auth

   https://github.com/cs3org/reva/issues/2171
   https://github.com/cs3org/reva/pull/2176

 * Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

 * Enhancement #2135: Nextcloud test improvements

   https://github.com/cs3org/reva/pull/2135

 * Enhancement #2180: Remove OCDAV options namespace parameter

   We dropped the namespace parameter, as it is not used in the options handler.

   https://github.com/cs3org/reva/pull/2180

 * Enhancement #2117: Add ocs cache warmup strategy for first request from the user

   https://github.com/cs3org/reva/pull/2117

 * Enhancement #2170: Handle propfind requests for existing files

   https://github.com/cs3org/reva/pull/2170

 * Enhancement #2165: Allow access to recycle bin for arbitrary paths outside homes

   https://github.com/cs3org/reva/pull/2165
   https://github.com/cs3org/reva/pull/2188

 * Enhancement #2193: Filter root paths according to user agent

   Adds a new rule setting in the storage registry ("allowed_user_agents"), that allows a user to
   specify which storage provider shows according to the user agent that made the request.

   https://github.com/cs3org/reva/pull/2193

 * Enhancement #2162: Implement the UpdateStorageSpace method

   Added the UpdateStorageSpace method to the decomposedfs.

   https://github.com/cs3org/reva/pull/2162
   https://github.com/cs3org/reva/pull/2195
   https://github.com/cs3org/reva/pull/2196

 * Enhancement #2189: Add user setting capability

   We've added a capability to communicate the existance of a user settings service to clients.

   https://github.com/owncloud/web/issues/5926
   https://github.com/cs3org/reva/pull/2189
   https://github.com/owncloud/ocis/pull/2655


