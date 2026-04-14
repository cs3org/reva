
---
title: "v3.7.0"
linkTitle: "v3.7.0"
weight: 999630
description: >
  Changelog for Reva v3.7.0 (2026-04-14)
---

Changelog for reva 3.7.0 (2026-04-14)
=======================================

The following sections list the changes in reva 3.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Enh #5553: Make ListShares conurrent
 * Enh #5554: Make ListStorageSpaces concurrent
 * Enh #5569: Add processing of ro-crates
 * Enh #5526: Do not impersonate owner for public link access
 * Enh #5514: Refactor EOSFS auth logic
 * Enh #5550: Add span collection to tracing interceptor

Details
-------

 * Enhancement #5553: Make ListShares conurrent

   https://github.com/cs3org/reva/pull/5553

 * Enhancement #5554: Make ListStorageSpaces concurrent

   https://github.com/cs3org/reva/pull/5554

 * Enhancement #5569: Add processing of ro-crates

   This is done via the UpdateReceivedShareCall using an opaque and we want to be able to have a
   seperate CS3API call for this in the future

   https://github.com/cs3org/reva/pull/5569

 * Enhancement #5526: Do not impersonate owner for public link access

   Instead of impersonating the owner of a public link, we create a Guest User which has
   permissions through an added scope

   https://github.com/cs3org/reva/pull/5526

 * Enhancement #5514: Refactor EOSFS auth logic

   - Use a dedicated service account for accesses made by external accounts, instead of
   impersonating the owner or using a token - Renamed the different types of auth to be more clear
   (e.g. cboxAuth became systemAuth) - Added a `InvalidAuthorization` to be returned instead of
   an empty auth; because empty auth maps to the system user (which is a sudo'er)

   https://github.com/cs3org/reva/pull/5514

 * Enhancement #5550: Add span collection to tracing interceptor

   This allows us to view full spans of calls in jaeger / otel tracing tools

   https://github.com/cs3org/reva/pull/5550/


