
---
title: "v3.4.0"
linkTitle: "v3.4.0"
weight: 999660
description: >
  Changelog for Reva v3.4.0 (2025-12-12)
---

Changelog for reva 3.4.0 (2025-12-12)
=======================================

The following sections list the changes in reva 3.4.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5425: Make public file PROPFIND response proper again
 * Fix #5421: Eosfs: SetLock now checks existing locks
 * Fix #5424: Make href properly formed on PROPFIND requests to /v endpoints
 * Fix #5418: Fix inverted expiry check in JSON invite repository
 * Fix #5412: Nilpointer in logline when addGrant fails
 * Fix #5415: Set correct user type when accepting OCM invites
 * Fix #5422: Do not give delete permission to root of public links
 * Fix #5409: Bring back proper DAV support
 * Fix #5430: Fix trashbin restores
 * Fix #5427: Fix upload notifications
 * Fix #5423: Make href proper in REPORT
 * Enh #5385: Add OCM Where Are You From capability
 * Enh #5403: Add Acess-Control-Expose-Headers for Range requests
 * Enh #5323: Add database config to sharedconf
 * Enh #5432: Add support for embedded view mode in apps
 * Enh #5408: Modernize codebase
 * Enh #5407: Add musl-based fully static build target
 * Enh #5401: Adapt gateway and corresponding driver to CS3API changes
 * Enh #5363: Include OCM shares in SharedByMe view
 * Enh #5285: Remove EOS EnableHome parameter
 * Enh #5405: Add support for signed URLs
 * Enh #5381: Convert SQL tables to gorm, corresponding driver, and tests

Details
-------

 * Bugfix #5425: Make public file PROPFIND response proper again

   https://github.com/cs3org/reva/pull/5425

 * Bugfix #5421: Eosfs: SetLock now checks existing locks

   https://github.com/cs3org/reva/pull/5421

 * Bugfix #5424: Make href properly formed on PROPFIND requests to /v endpoints

   https://github.com/cs3org/reva/pull/5424

 * Bugfix #5418: Fix inverted expiry check in JSON invite repository

   The `tokenIsExpired` function in the JSON invite repository had the comparison operator
   inverted, causing valid (non-expired) tokens to be incorrectly filtered out when listing
   invite tokens.

   The check `token.Expiration.Seconds > Now()` was returning true for tokens expiring in the
   future, effectively hiding all valid tokens. Fixed to use `<` instead of `>`.

   https://github.com/cs3org/reva/pull/5418

 * Bugfix #5412: Nilpointer in logline when addGrant fails

   Fix for https://github.com/cs3org/reva/issues/5387

   https://github.com/cs3org/reva/pull/5412

 * Bugfix #5415: Set correct user type when accepting OCM invites

   When a remote user accepts an OCM invite, they were being stored with USER_TYPE_PRIMARY
   instead of USER_TYPE_FEDERATED. This caused federated user searches to fail and OCM share
   creation to break because the user ID was not properly formatted with the @domain suffix
   required for OCM address resolution.

   https://github.com/cs3org/reva/pull/5415

 * Bugfix #5422: Do not give delete permission to root of public links

   Otherwise, a folder shared through a public link could itself be deleted

   https://github.com/cs3org/reva/pull/5422

 * Bugfix #5409: Bring back proper DAV support

   Spaces broke proper DAV support, because returned hrefs in the PROPFIND always contained
   space IDs, even if these were not present in the incoming request. This is fixed now, by writing
   the href based in the incoming URL

   https://github.com/cs3org/reva/pull/5409

 * Bugfix #5430: Fix trashbin restores

   https://github.com/cs3org/reva/pull/5430

 * Bugfix #5427: Fix upload notifications

   The registration of notifications for uploads in a public link folder was until now only
   handled in the OCS HTTP layer; this is the responsibility of the public share provider. Since it
   was also missing from the OCGraph layer, this has been moved to the "gRPC" part of reva

   https://github.com/cs3org/reva/pull/5427

 * Bugfix #5423: Make href proper in REPORT

   Github.com/cs3org/reva/pull/5409 broke REPORT calls, which are used for favorites. This is
   now fixed

   https://github.com/cs3org/reva/pull/5423

 * Enhancement #5385: Add OCM Where Are You From capability

   Implements WAYF specific discovery endpoints for the ScienceMesh package, enabling dynamic
   OCM provider discovery and federation management.

   https://github.com/cs3org/reva/pull/5385

 * Enhancement #5403: Add Acess-Control-Expose-Headers for Range requests

   We add the necessary headers for multipart range requests to Acess-Control-Expose-Headers
   to expose these, so that clients can read them

   https://github.com/cs3org/reva/pull/5403

 * Enhancement #5323: Add database config to sharedconf

   Add database configuration to `sharedconf`, so that it doesn't have to be repeated for every
   driver

   https://github.com/cs3org/reva/pull/5323

 * Enhancement #5432: Add support for embedded view mode in apps

   https://github.com/cs3org/reva/pull/5432

 * Enhancement #5408: Modernize codebase

   This PR
   [modernize](https://pkg.go.dev/golang.org/x/tools/gopls/internal/analysis/modernize)s
   the codebase: it removes syntax that used to be idiomatic but now has better alternatives

   https://github.com/cs3org/reva/pull/5408

 * Enhancement #5407: Add musl-based fully static build target

   Added a new `revad-static-musl` Makefile target that produces a fully statically linked
   binary using musl libc instead of glibc. This eliminates the linker warnings that appeared
   with the standard static build and creates a truly portable binary that runs on any Linux
   distribution without requiring matching glibc versions.

   Also fixed the build info injection by correcting the package path in BUILD_FLAGS to include
   the `/v3` module version, ensuring version, commit, and build date information are properly
   displayed in the binary.

   https://github.com/cs3org/reva/pull/5407

 * Enhancement #5401: Adapt gateway and corresponding driver to CS3API changes

   The OCM Core API has been renamed to OCM Incoming API

   https://github.com/cs3org/reva/pull/5401

 * Enhancement #5363: Include OCM shares in SharedByMe view

   - The CS3APis verison has been updated to include "ListExistingOcmShares". - The OCM shares
   are now included in the getSharedByMe call. - The filters have been updated to adapt to changes
   from the updated CS3APIs. - Fixed bug where only ocm users were queried if it was enabled. -
   Consolidated OCM Address resolutions in a single function, fixes #5383

   https://github.com/cs3org/reva/pull/5363

 * Enhancement #5285: Remove EOS EnableHome parameter

   This change removes the `EnableHome` parameter, which was a source of bugs and was unused in
   production.

   https://github.com/cs3org/reva/pull/5285

 * Enhancement #5405: Add support for signed URLs

   https://github.com/cs3org/reva/pull/5405

 * Enhancement #5381: Convert SQL tables to gorm, corresponding driver, and tests

   - Conversion of the SQL tables to a GORM model, IDs are unique across public links, normal
   shares, and OCM shares. - Some refactoring of the OCM tables (protocols and access methods) -
   Corresponding SQL driver for access has been implemented using GORM - Tests with basic
   coverage have been implemented.

   https://github.com/cs3org/reva/pull/5381


