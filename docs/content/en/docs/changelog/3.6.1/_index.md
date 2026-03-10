
---
title: "v3.6.1"
linkTitle: "v3.6.1"
weight: 999639
description: >
  Changelog for Reva v3.6.1 (2026-03-10)
---

Changelog for reva 3.6.1 (2026-03-10)
=======================================

The following sections list the changes in reva 3.6.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5521: Fix issue where permissions is on subdir
 * Fix #5524: Fix inverted logic in pathToSpaceID
 * Fix #5520: Fix issues with sharing for the cephmount driver
 * Enh #5519: Support additional gids in cephmount fs driver
 * Enh #5525: Support getting quota from path ancestors
 * Enh #5529: Improves the GetQuota call
 * Enh #5527: Make ceph version with local plugins
 * Enh #5523: Add status field to projects table
 * Enh #5518: Clean up how space IDs are calculated

Details
-------

 * Bugfix #5521: Fix issue where permissions is on subdir

   * This PR fixes a bug where the permissions are on a subdirectory and the admin mount fails.

   https://github.com/cs3org/reva/pull/5521

 * Bugfix #5524: Fix inverted logic in pathToSpaceID

   Fixed a bug where the condition for falling back to the mount_path when no proper space depth was
   set was inverted.

   https://github.com/cs3org/reva/pull/5524

 * Bugfix #5520: Fix issues with sharing for the cephmount driver

   This PR includes two fixes * ListVersions now returns an empty list. * The root directory is
   appended to the CephVolumePath.

   https://github.com/cs3org/reva/pull/5520

 * Enhancement #5519: Support additional gids in cephmount fs driver

   When accessing the underlying Ceph mount, the linux thread only contained the uid/gid of the
   user. Now it also contaisn the additional gids of the user, which allows to access files that are
   only accessible by group permissions.

   https://github.com/cs3org/reva/pull/5519

 * Enhancement #5525: Support getting quota from path ancestors

   The previous code supported to obtained quota only from the path itself, but in some cases, the
   path may not contain the quota information, but its ancestor may contain the quota
   information.

   https://github.com/cs3org/reva/pull/5525

 * Enhancement #5529: Improves the GetQuota call

   The ceph.dir.rbytes exists at every level and should be checked at the level where the quota is
   found

   https://github.com/cs3org/reva/pull/5529

 * Enhancement #5527: Make ceph version with local plugins

   Add the option to build the ceph version of reva with local plugins

   https://github.com/cs3org/reva/pull/5527

 * Enhancement #5523: Add status field to projects table

   A new status field has been added to the projects table with possible values: creating, active,
   and archived. The default value is 'active'. ListStorageSpaces now accepts a status
   parameter to filter projects by their status. A new method UpdateProjectStatus allows
   changing the status of a project.

   https://github.com/cs3org/reva/pull/5523

 * Enhancement #5518: Clean up how space IDs are calculated

   Instead of having hardcoded paths, this is now configurable through a `space_depth`
   parameter in the storage provider

   https://github.com/cs3org/reva/pull/5518


