
---
title: "v3.10.0"
linkTitle: "v3.10.0"
weight: 999600
description: >
  Changelog for Reva v3.10.0 (2026-06-03)
---

Changelog for reva 3.10.0 (2026-06-03)
=======================================

The following sections list the changes in reva 3.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5624: Throw proper err upon invalid COPY request
 * Fix #5620: Fix PROPFIND inconsistencies
 * Enh #5625: Store attributes on version folders in EOS
 * Enh #5627: Use destination field in received ocm share
 * Enh #5626: Implement CS3 labels API
 * Enh #5638: Add SpaceID field in shares table
 * Enh #5629: Support for zenodo json-ld wrapped ro-crates

Details
-------

 * Bugfix #5624: Throw proper err upon invalid COPY request

   When doing a COPY / MOVE with an invalid space id, Reva starts copying from `/` instead of
   returning an error

   https://github.com/cs3org/reva/pull/5624

 * Bugfix #5620: Fix PROPFIND inconsistencies

   Stop emitting a duplicate `oc:public-link-expiration` entry in the not-found list, drop an
   erroneous fallthrough from `oc:privatelink` and skip re-adding `oc:name` when explicitly
   requested (since it's already added unconditionally)

   https://github.com/cs3org/reva/pull/5620

 * Enhancement #5625: Store attributes on version folders in EOS

   See ADR decisions/reva/0004-attributes-version-folder.md

   https://github.com/cs3org/reva/pull/5625

 * Enhancement #5627: Use destination field in received ocm share

   Instead of hijacking the opaque we use the new destination field when processing a share

   https://github.com/cs3org/reva/pull/5627

 * Enhancement #5626: Implement CS3 labels API

   Together with this, the new table is also "GORM-ified"

   https://github.com/cs3org/reva/pull/5626

 * Enhancement #5638: Add SpaceID field in shares table

   On top of that, we set the owner of a share to be initiator

   https://github.com/cs3org/reva/pull/5638

 * Enhancement #5629: Support for zenodo json-ld wrapped ro-crates

   Adds support for OCM embedded shares whose payload is an RO-Crate wrapping the Zenodo JSON-LD
   format (the plain RO-Crate is also supported)

   https://github.com/cs3org/reva/pull/5629


