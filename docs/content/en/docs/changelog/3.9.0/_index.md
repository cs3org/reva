
---
title: "v3.9.0"
linkTitle: "v3.9.0"
weight: 999610
description: >
  Changelog for Reva v3.9.0 (2026-05-21)
---

Changelog for reva 3.9.0 (2026-05-21)
=======================================

The following sections list the changes in reva 3.9.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5619: Fix EOS bug in version folder creation
 * Fix #5618: Workaround for weird EOS ENODATA issue
 * Fix #5610: Display ResourceType correctly for OCM Shares
 * Fix #5609: OCM invite manager should use shared DB conf
 * Enh #5571: Exclude expired shares from `ListReceivedShares`
 * Enh #5552: OCM code-flow token exchange
 * Enh #5598: Performance improvements to /permissions call
 * Enh #5613: Check verb in signed URLs

Details
-------

 * Bugfix #5619: Fix EOS bug in version folder creation

   Due to a bug in the EOS drivers, version folders were created under the owner of the first
   resource in a directory, instead of the owner of the corresponding file.

   https://github.com/cs3org/reva/pull/5619

 * Bugfix #5618: Workaround for weird EOS ENODATA issue

   https://github.com/cs3org/reva/pull/5618

 * Bugfix #5610: Display ResourceType correctly for OCM Shares

   https://github.com/cs3org/reva/pull/5610

 * Bugfix #5609: OCM invite manager should use shared DB conf

   https://github.com/cs3org/reva/pull/5609

 * Enhancement #5571: Exclude expired shares from `ListReceivedShares`

   - `ListReceivedShares` no longer returns shares whose expiration date has passed

   https://github.com/cs3org/reva/pull/5571

 * Enhancement #5552: OCM code-flow token exchange

   Added end-to-end OCM code-flow support for both sender and receiver paths. Shares can now
   declare `requirements: ["must-exchange-token"]` and `accessTypes: ["remote"]`, with the
   new fields preserved across both SQL and JSON persistence. Discovery advertises
   `tokenEndPoint` and `exchange-token`, the new `POST /ocm/token` endpoint exchanges
   authorization codes into short-lived JWTs, and the dedicated `ocmsharecode` and
   `ocmexchangedtoken` auth managers separate exchange-code validation from
   exchanged-token validation.

   The same feature branch also hardens the runtime path uncovered during live interop
   validation. Code-flow scopes now carry share and resource identity without embedding the
   long-lived shared secret, malformed protocol payloads are rejected earlier, and the
   validated interop fixes stay in the same change set: correct `client_id` handling,
   root-mounted DAV share recovery for Nextcloud-style clients, preserved download paths for
   root-mounted single-file reads, and the related WOPI external-link fix that prefers
   canonical share ids over legacy tokens.

   Validation coverage was expanded at the seams that changed most. The branch now includes
   focused tests for `/ocm/token` behavior, discovery-to-route coupling, WOPI share-id
   fallback, received-side token-endpoint discovery and exchange helpers, received-side
   retry wrappers, and persistence/validation behavior. Legacy direct-secret flows remain
   operational, while code-flow shares enforce token exchange as an explicit protocol
   requirement.

   https://github.com/cs3org/reva/pull/5552

 * Enhancement #5598: Performance improvements to /permissions call

   https://github.com/cs3org/reva/pull/5598

 * Enhancement #5613: Check verb in signed URLs

   https://github.com/cs3org/reva/pull/5613


