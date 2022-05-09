
---
title: "v2.3.1"
linkTitle: "v2.3.1"
weight: 40
description: >
  Changelog for Reva v2.3.1 (2022-05-08)
---

Changelog for reva 2.3.1 (2022-05-08)
=======================================

The following sections list the changes in reva 2.3.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2827: Check permissions when deleting spaces
 * Fix #2830: Correctly render response when accepting merged shares
 * Fix #2831: Fix uploads to owncloudsql storage when no mtime is provided
 * Enh #2833: Make status.php values configurable
 * Enh #2832: Add version option for ocdav go-micro service

Details
-------

 * Bugfix #2827: Check permissions when deleting spaces

   Do not allow viewers and editors to delete a space (you need to be manager) Block deleting a space
   via dav service (should use graph to avoid accidental deletes)

   https://github.com/cs3org/reva/pull/2827

 * Bugfix #2830: Correctly render response when accepting merged shares

   We now only return the data for the accepted share instead of concatenating data for all
   affected shares.

   https://github.com/cs3org/reva/pull/2830

 * Bugfix #2831: Fix uploads to owncloudsql storage when no mtime is provided

   We've fixed uploads to owncloudsql storage when no mtime is provided. We now just use the
   current timestamp. Previously the upload did fail.

   https://github.com/cs3org/reva/pull/2831

 * Enhancement #2833: Make status.php values configurable

   We've added an option to set the status values for `product`, `productname`, `version`,
   `versionstring` and `edition`.

   https://github.com/cs3org/reva/pull/2833

 * Enhancement #2832: Add version option for ocdav go-micro service

   We've added an option to set a version for the ocdav go-micro registry. This enables you to set a
   version queriable by from the go-micro registry.

   https://github.com/cs3org/reva/pull/2832


