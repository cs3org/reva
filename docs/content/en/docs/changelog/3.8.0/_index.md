
---
title: "v3.8.0"
linkTitle: "v3.8.0"
weight: 999620
description: >
  Changelog for Reva v3.8.0 (2026-04-29)
---

Changelog for reva 3.8.0 (2026-04-29)
=======================================

The following sections list the changes in reva 3.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5515: Cephmount: added more fail-safe cases at init time
 * Enh #5558: Appprovider: handle a force-viewmode-reason value
 * Enh #5556: Allow using different field as identifier, configurable per idp

Details
-------

 * Bugfix #5515: Cephmount: added more fail-safe cases at init time

   This PR adds more cases where cephmount would bail out at init time when preconditions are not
   met.

   https://github.com/cs3org/reva/pull/5515

 * Enhancement #5558: Appprovider: handle a force-viewmode-reason value

   Paired with https://github.com/cs3org/wopiserver/pull/195

   https://github.com/cs3org/reva/pull/5558

 * Enhancement #5556: Allow using different field as identifier, configurable per idp

   https://github.com/cs3org/reva/pull/5556


