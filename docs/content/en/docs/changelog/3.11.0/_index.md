
---
title: "v3.11.0"
linkTitle: "v3.11.0"
weight: 999590
description: >
  Changelog for Reva v3.11.0 (2026-07-07)
---

Changelog for reva 3.11.0 (2026-07-07)
=======================================

The following sections list the changes in reva 3.11.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5693: Bugfix where descriptions are not propagated to web
 * Fix #5694: Whitelisted more request types for the lightweight scope
 * Chg #5671: Dropped support for Nextcloud as storage/user/auth provider
 * Enh #5691: Refactor current ocm invite manager to GORM
 * Enh #5688: Modernize Go Code
 * Enh #5562: Share hierarchy checks
 * Enh #5690: Store remote users on ocm share

Details
-------

 * Bugfix #5693: Bugfix where descriptions are not propagated to web

   This fixes a bug where the descriptions of the ocm invites are not propagated to the web.

   https://github.com/cs3org/reva/pull/5693

 * Bugfix #5694: Whitelisted more request types for the lightweight scope

   This is required to enable OCM for external accounts

   https://github.com/cs3org/reva/pull/5694

 * Change #5671: Dropped support for Nextcloud as storage/user/auth provider

   This PR removes the code to support Nextcloud as storage, user, and auth provider.

   This code was developed as part of the initial effort to put in place the ScienceMesh, where the
   deployment model was to run Reva at each site, including sites running Nextcloud where Reva
   would be responsible for the OCM-based federation layer and Nextcloud for all the rest.

   Over the years, and especially during 2025-26, Nextcloud has implemented all OCM-related
   capabilities natively, and this interface is getting obsoleted. We have kept it until the
   maintenance cost was negligible, but with the upcoming changes on the OCM implementation,
   this is not sustainable any longer.

   https://github.com/cs3org/reva/pull/5671

 * Enhancement #5691: Refactor current ocm invite manager to GORM

   Refactor the ocm invite manager to user the GORM.

   https://github.com/cs3org/reva/pull/5691

 * Enhancement #5688: Modernize Go Code

   https://github.com/cs3org/reva/pull/5688

 * Enhancement #5562: Share hierarchy checks

   This PR adds a hierarchical checking algorithm for shares to the gateway, as defined in ADR
   general/0005-sharing. Concrectely, the new algorithm does the following:

   * Before applying any ACL, the gateway checks for parent and child shares in the database. *
   Based on their relationships and permission levels, the gateway decides whether to apply,
   reapply, or reject the operation. * ACL updates will be applied orderd by path-length (where
   the shortest comes first) to maintain consistent inheritance semantics (otherwise, you
   would overwrite child shares). * The algorithm applies equally to create, update, and delete
   operations.

   https://github.com/cs3org/reva/pull/5562

 * Enhancement #5690: Store remote users on ocm share

   This commit adds the following functionality:

   - Functionality to add remote users even without the invitation flow when an OCM share is
   received. - This configuration option is a whitelist of hosts of which this functionality is
   enabled for. - A machine secret is added so that we can impersonate the user receiving the share
   since the ocm endpoint is unauthenticated.

   https://github.com/cs3org/reva/pull/5690


