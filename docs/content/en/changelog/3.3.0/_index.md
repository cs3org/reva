
---
title: "v3.3.0"
linkTitle: "v3.3.0"
weight: 40
description: >
  Changelog for Reva v3.3.0 (2025-10-20)
---

Changelog for reva 3.3.0 (2025-10-20)
=======================================

The following sections list the changes in reva 3.3.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5338: Fix OCM legacy access
 * Fix #5364: Remove trash and versions from OCS role
 * Fix #5366: Add support for Deny Role
 * Fix #5365: Skip spaces that are not available
 * Enh #5367: Proper support of Range requests

Details
-------

 * Bugfix #5338: Fix OCM legacy access

   This PR introduces a test to access a remote OCM endpoint via basic auth (OCM v1.0) and the
   corresponding implementation in the DAV client and server to deal with such accesses. Several
   log lines on all OCM interactions have been added.

   https://github.com/cs3org/reva/pull/5338

 * Bugfix #5364: Remove trash and versions from OCS role

   Remove trashbin and version-related permissions from conversion to OCS role, as some space
   types do not support these, leading to invalid roles

   https://github.com/cs3org/reva/pull/5364

 * Bugfix #5366: Add support for Deny Role

   In OCS, we had a `RoleDenied`, which denied all permissions to a user. We now also ported this to
   libregraph.

   https://github.com/cs3org/reva/pull/5366

 * Bugfix #5365: Skip spaces that are not available

   Skip spaces that are not available when listing them. This avoids WebUI hanging when one of them
   is not reachable.

   https://github.com/cs3org/reva/pull/5365

 * Enhancement #5367: Proper support of Range requests

   Up to now, Reva supported Range requests only when the requested content was copied into Reva's
   memory. This has now been improved: the ranges can be propagated upto the storage provider,
   which only returns the requested content, instead of first copying everything into memory and
   then only returning the requested ranges

   https://github.com/cs3org/reva/pull/5367


