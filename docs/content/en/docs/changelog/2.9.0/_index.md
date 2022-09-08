
---
title: "v2.9.0"
linkTitle: "v2.9.0"
weight: 40
description: >
  Changelog for Reva v2.9.0 (2022-09-08)
---

Changelog for reva 2.9.0 (2022-09-08)
=======================================

The following sections list the changes in reva 2.9.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3206: Add spaceid when listing share jail mount points
*   Fix #3194: Adds the rootinfo to storage spaces
*   Fix #3201: Fix shareid on PROPFIND
*   Fix #3176: Forbid duplicate shares
*   Fix #3208: Prevent panic in time conversion
*   Fix #3881: Align ocs status code for permission error on publiclink update
*   Enh #3193: Add shareid to PROPFIND
*   Enh #3180: Add canDeleteAllHomeSpaces permission
*   Enh #3203: Added "delete-all-spaces" permission
*   Enh #4322: OCS get share now also handle received shares
*   Enh #3185: Improve ldap authprovider's error reporting
*   Enh #3179: Improve tokeninfo endpoint
*   Enh #3171: Cs3 to jsoncs3 share manager migration
*   Enh #3204: Make the function flockFile private
*   Enh #3192: Enable space members to update shares

Details
-------

*   Bugfix #3206: Add spaceid when listing share jail mount points

   https://github.com/cs3org/reva/pull/3206

*   Bugfix #3194: Adds the rootinfo to storage spaces

   The sympton of the bug were search results not containing permissions

   https://github.com/cs3org/reva/pull/3194

*   Bugfix #3201: Fix shareid on PROPFIND

   Shareid was still not working properly. We need to parse it from the path

   https://github.com/cs3org/reva/pull/3201

*   Bugfix #3176: Forbid duplicate shares

   When sending a CreateShare request twice two shares would be created, one being not
   accessible. This was blocked by web so the issue wasn't obvious. Now it's forbidden to create
   share for a user who already has a share on that same resource

   https://github.com/cs3org/reva/pull/3176

*   Bugfix #3208: Prevent panic in time conversion

   https://github.com/cs3org/reva/pull/3208

*   Bugfix #3881: Align ocs status code for permission error on publiclink update

   The ocs status code returned for permission errors on updates of publiclink permissions is now
   aligned with the documentation of the OCS share API and the behaviour of ownCloud 10

   https://github.com/owncloud/ocis/issues/3881

*   Enhancement #3193: Add shareid to PROPFIND

   Adds the shareid to the PROPFIND response (in case of shares only)

   https://github.com/cs3org/reva/pull/3193

*   Enhancement #3180: Add canDeleteAllHomeSpaces permission

   We added a permission to the admin role in ocis that allows deleting homespaces on user delete.

   https://github.com/cs3org/reva/pull/3180
   https://github.com/cs3org/reva/pull/3202
   https://github.com/owncloud/ocis/pull/4447/files

*   Enhancement #3203: Added "delete-all-spaces" permission

   We introduced a new permission "delete-all-spaces", users holding this permission are
   allowed to delete any space of any type.

   https://github.com/cs3org/reva/pull/3203

*   Enhancement #4322: OCS get share now also handle received shares

   Requesting a specific share can now also correctly map the path to the mountpoint if the
   requested share is a received share.

   https://github.com/owncloud/ocis/issues/4322
   https://github.com/cs3org/reva/pull/3200

*   Enhancement #3185: Improve ldap authprovider's error reporting

   The errorcode returned by the ldap authprovider driver is a bit more explicit now. (i.e. we
   return a proper Invalid Credentials error now, when the LDAP Bind operation fails with that)

   https://github.com/cs3org/reva/pull/3185

*   Enhancement #3179: Improve tokeninfo endpoint

   We added more information to the tokeninfo endpoint. `aliaslink` is a bool value indicating if
   the permissions are 0. `id` is the full id of the file. Both are available to all users having the
   link token. `spaceType` (indicating the space type) is only available if the user has native
   access

   https://github.com/cs3org/reva/pull/3179

*   Enhancement #3171: Cs3 to jsoncs3 share manager migration

   We added a Load() to the jsoncs3 and Dump() to the sc3 share manager. The shareid might need to be
   prefixed with a storageid and space id.

   https://github.com/cs3org/reva/pull/3171
   https://github.com/cs3org/reva/pull/3195

*   Enhancement #3204: Make the function flockFile private

   Having that function exported is tempting people to use the func to get the name for calling the
   lock functions. That is wrong, as this function is just a helper to generate the lock file name
   from a given file to lock.

   https://github.com/cs3org/reva/pull/3204

*   Enhancement #3192: Enable space members to update shares

   Enabled space members to update shares which they have not created themselves.

   https://github.com/cs3org/reva/pull/3192

