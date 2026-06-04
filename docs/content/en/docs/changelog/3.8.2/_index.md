
---
title: "v3.8.2"
linkTitle: "v3.8.2"
weight: 999618
description: >
  Changelog for Reva v3.8.2 (2026-05-15)
---

Changelog for reva 3.8.2 (2026-05-15)
=======================================

The following sections list the changes in reva 3.8.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Enh: HTTP auth middleware returns machine-readable 409 for identity conflicts (linked primary / lightweight linked to primary)

Details
-------

 * Enhancement: Linked primary identity conflict HTTP contract

   When gateway `Authenticate` fails with GRPC `CODE_ABORTED` (auth manager `errtypes.Conflict`, e.g. OIDC user resolution),
   the HTTP auth interceptor now responds with **409 Conflict**, header **`X-Oc-Linked-Primary-Account: true`**, header
   **`Cache-Control: no-store`**, and a Libre Graph-style JSON body (`error.code` = `linkedPrimaryAccount`) so ownCloud Web
   and other clients can surface a dedicated sign-in blocked experience.

   See [Identity auth HTTP errors]({{< relref "/docs/identity-auth-http-errors" >}}).

