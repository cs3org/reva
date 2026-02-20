
---
title: "v3.6.0"
linkTitle: "v3.6.0"
weight: 999640
description: >
  Changelog for Reva v3.6.0 (2026-02-20)
---

Changelog for reva 3.6.0 (2026-02-20)
=======================================

The following sections list the changes in reva 3.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5451: Handle special characters in file names correctly
 * Fix #5511: Quota should be retrieved in monitoring key=value format
 * Enh #5481: Add support for reva-cli tests on EOS
 * Enh #5475: Support project-trashbins in EOS
 * Enh #5472: Use uid instead of username of eos remote-user
 * Enh #5474: Update EOS gRPC bindings
 * Enh #5513: Refactor EOS driver / FS for Reva
 * Enh #5503: Cephmount add root dir config option

Details
-------

 * Bugfix #5451: Handle special characters in file names correctly

   - Fixed PROPFIND response when listing a folder named with special charactrers

   https://github.com/cs3org/reva/pull/5451

 * Bugfix #5511: Quota should be retrieved in monitoring key=value format

   The quota command now uses the `-m` flag to retrieve quota information in monitoring key=value
   format, ensuring consistent and parseable output.

   https://github.com/cs3org/reva/pull/5511

 * Enhancement #5481: Add support for reva-cli tests on EOS

   https://github.com/cs3org/reva/pull/5481

 * Enhancement #5475: Support project-trashbins in EOS

   https://github.com/cs3org/reva/pull/5475

 * Enhancement #5472: Use uid instead of username of eos remote-user

   This brings back the functionality to download versions of files in a project space

   https://github.com/cs3org/reva/pull/5472

 * Enhancement #5474: Update EOS gRPC bindings

   This supports the new trashbin, based on a "recycle id" for projects

   https://github.com/cs3org/reva/pull/5474

 * Enhancement #5513: Refactor EOS driver / FS for Reva

   This also includes writing a README for the driver

   https://github.com/cs3org/reva/pull/5513

 * Enhancement #5503: Cephmount add root dir config option

   * The `root_dir` config option is there to append a path on the current path that should be
   mounted by reva

   https://github.com/cs3org/reva/pull/5503


