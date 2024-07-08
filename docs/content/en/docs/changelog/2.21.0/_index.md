
---
title: "v2.21.0"
linkTitle: "v2.21.0"
weight: 40
description: >
  Changelog for Reva v2.21.0 (2024-07-08)
---

Changelog for reva 2.21.0 (2024-07-08)
=======================================

The following sections list the changes in reva 2.21.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4740: Disallow reserved filenames
*   Fix #4748: Quotes in dav Content-Disposition header
*   Fix #4750: Validate a space path
*   Enh #4737: Add the backchannel logout event
*   Enh #4749: DAV error codes
*   Enh #4742: Expose disable-versioning configuration option
*   Enh #4739: Improve posixfs storage driver
*   Enh #4738: Add GetServiceUserToken() method to utils pkg

Details
-------

*   Bugfix #4740: Disallow reserved filenames

   We now disallow the reserved `..` and `.` filenames. They are only allowed as destinations of
   move or copy operations.

   https://github.com/cs3org/reva/pull/4740

*   Bugfix #4748: Quotes in dav Content-Disposition header

   We've fixed the the quotes in the dav `Content-Disposition` header. They caused an issue where
   certain browsers would decode the quotes and falsely prepend them to the filename.

   https://github.com/owncloud/web/issues/11031
   https://github.com/cs3org/reva/pull/4748

*   Bugfix #4750: Validate a space path

   We've fixed the issue when validating a space path

   https://github.com/cs3org/reva/pull/4750
   https://github.com/cs3org/reva/pull/4753

*   Enhancement #4737: Add the backchannel logout event

   We've added the backchannel logout event

   https://github.com/owncloud/ocis/issues/9355
   https://github.com/cs3org/reva/pull/4737

*   Enhancement #4749: DAV error codes

   DAV error responses now include an error code for clients to use if they need to check for a
   specific error type.

   https://github.com/owncloud/ocis/issues/9533
   https://github.com/cs3org/reva/pull/4749

*   Enhancement #4742: Expose disable-versioning configuration option

   This PR exposes the disable-versioning configuration option to the user. This option allows
   the user to disable versioning for the storage-providers.

   https://github.com/cs3org/reva/pull/4742

*   Enhancement #4739: Improve posixfs storage driver

   Improve the posixfs storage driver by fixing several issues and adding missing features.

   https://github.com/cs3org/reva/pull/4739

*   Enhancement #4738: Add GetServiceUserToken() method to utils pkg

   Added GetServiceUserToken() function to the utils pkg to easily get a reva token for a service
   account.

   https://github.com/cs3org/reva/pull/4738

