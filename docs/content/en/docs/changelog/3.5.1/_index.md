
---
title: "v3.5.1"
linkTitle: "v3.5.1"
weight: 999649
description: >
  Changelog for Reva v3.5.1 (2026-02-12)
---

Changelog for reva 3.5.1 (2026-02-12)
=======================================

The following sections list the changes in reva 3.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5482: Return 403 instead of 500 on no permissions
 * Fix #5473: Do not require db_engine to be set
 * Fix #5496: GetNsMatch in eosfs should return a cleaned path
 * Fix #5490: Do not do getQuota as project owner for ownerless projects
 * Fix #5492: Set proper service account context for project GetQuota
 * Fix #5491: Do not clean namespace path
 * Enh #5466: Clean up DAV PUT
 * Enh #5470: Add indexes to share db for fields that may be queried
 * Enh #5452: OCM Embedded shares
 * Enh #5471: Extend integration tests using Reva CLI
 * Enh #5476: OCM: support access_types and drop datatx protocol
 * Enh #5489: Add processing endpoint for an embedded share
 * Enh #5484: Remove share first from EOS, then db
 * Enh #5462: Integration tests using Reva CLI

Details
-------

 * Bugfix #5482: Return 403 instead of 500 on no permissions

   https://github.com/cs3org/reva/pull/5482

 * Bugfix #5473: Do not require db_engine to be set

   https://github.com/cs3org/reva/pull/5473

 * Bugfix #5496: GetNsMatch in eosfs should return a cleaned path

   https://github.com/cs3org/reva/pull/5496

 * Bugfix #5490: Do not do getQuota as project owner for ownerless projects

   https://github.com/cs3org/reva/pull/5490

 * Bugfix #5492: Set proper service account context for project GetQuota

   https://github.com/cs3org/reva/pull/5492

 * Bugfix #5491: Do not clean namespace path

   There was a bug in Reva that caused the namespace path to be cleaned. This is wrong: this path
   should end in a `/` so that EOS trashbin paths do not match for projects (e.g.
   `/eos/project-i00` should not be prefixed by `/eos/project/`).

   This bug caused deleted entries to still show up in the favorites list

   https://github.com/cs3org/reva/pull/5491

 * Enhancement #5466: Clean up DAV PUT

   * Fixed a "bug" where a variable was overwritten (though this had no impact) * Cleaned up a method
   * Removed unused function

   https://github.com/cs3org/reva/pull/5466

 * Enhancement #5470: Add indexes to share db for fields that may be queried

   In a previous change, some indexes for fields that are queried had been replaced by a composite
   index, which cannot be used for certain queries that we do. So, this PR brings back the
   non-composite indexes.

   https://github.com/cs3org/reva/pull/5470

 * Enhancement #5452: OCM Embedded shares

   This PR introduces OCM embedded shares

   * Adds functionality to store embedded shares (where the shared data is embedded in the OCM
   share payload) * Adds filters to `ListReceivedOCMShares` call and adapts to the new fields
   `SharedResourceType` and `RecipientType` * Adds an endpoint to list embedded shares (using
   the previously mentioned filters)

   https://github.com/cs3org/reva/pull/5452

 * Enhancement #5471: Extend integration tests using Reva CLI

   This PR extends the Reva CLI test suite by including tests for recycle bin, versions and grant
   operations

   https://github.com/cs3org/reva/pull/5471

 * Enhancement #5476: OCM: support access_types and drop datatx protocol

   This PR adapts the OCM implementation to v1.3. No new capabilities have been added yet.

   https://github.com/cs3org/reva/pull/5476

 * Enhancement #5489: Add processing endpoint for an embedded share

   - For now this only changes the state to accepted (or pending if you want to "unprocess" the
   share) - Eventually we want the downloaded content to arrive in the target folder, the target is
   just logged for now.

   https://github.com/cs3org/reva/pull/5489

 * Enhancement #5484: Remove share first from EOS, then db

   When removing shares, remove permissions from storage before going to db, so that there can be
   no lingering permissions on EOS

   https://github.com/cs3org/reva/pull/5484

 * Enhancement #5462: Integration tests using Reva CLI

   This PR introduces a proof-of-concept for integration tests based on the reva CLI. The goal is
   to extend these tests, so that they can be used by EOS to check if their changes break our
   workflow.

   https://github.com/cs3org/reva/pull/5462


