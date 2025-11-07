
---
title: "v3.3.3"
linkTitle: "v3.3.3"
weight: 999667
description: >
  Changelog for Reva v3.3.3 (2025-11-07)
---

Changelog for reva 3.3.3 (2025-11-07)
=======================================

The following sections list the changes in reva 3.3.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5393: Correct content-length when downloading versions
 * Fix #5380: Nilpointer in getPermissionsByCs3Reference
 * Fix #5388: Error code not checked correctly in eosclient
 * Fix #5392: Fixed permissions for upload-only links
 * Enh #5394: Fixed some logging going to stderr

Details
-------

 * Bugfix #5393: Correct content-length when downloading versions

   This fix corrects a bug introduced with the implementation of range requests, in
   https://github.com/cs3org/reva/pull/5367, where the content-length header was not
   populated correctly when downloading versions of a file, resulting in 0b.

   https://github.com/cs3org/reva/pull/5393

 * Bugfix #5380: Nilpointer in getPermissionsByCs3Reference

   Fix for potential nilpointer: when an err is returned, the status can be nil

   https://github.com/cs3org/reva/pull/5380

 * Bugfix #5388: Error code not checked correctly in eosclient

   Eosclient was returning an error when it actually succeeded

   https://github.com/cs3org/reva/pull/5388

 * Bugfix #5392: Fixed permissions for upload-only links

   Pending a proper refactoring of the permissions model, this PR fixes the bug unveiled after
   merging #5364. Cf. also Jira CERNBOX-4127.

   https://github.com/cs3org/reva/pull/5392

 * Enhancement #5394: Fixed some logging going to stderr

   https://github.com/cs3org/reva/pull/5394


