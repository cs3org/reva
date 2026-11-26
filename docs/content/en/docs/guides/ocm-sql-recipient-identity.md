---
title: "OCM SQL recipient identity"
category: OCM
description: Qualify local accounts with an empty identity provider for ordinary OCM shares
---

A local account with an empty IdP can receive an ordinary OCM share in SQL
when the incoming service can verify the account in its configured local
UserProvider. The share keeps the account's opaque ID, IdP, tenant and user
type exactly as issued. Per-share list, read and update use the authenticated
identity. Directed contact actions require a populated IdP and an explicit
user type.

The setting defaults to off. Before enabling it on the incoming service,
qualify the deployment.

1. Verify the effective UserProvider route and the active authentication
   sources. A single JSON users and credentials file used by both services is
   one supported profile. Check for duplicate login names, duplicate opaque
   IDs and collisions between the opaque ID or username and another account's
   lookup keys. For other directories or separate sources, verify equivalent
   uniqueness and locality for each eligible account.
2. Log in through Gateway and check that WhoAmI returns the same opaque ID,
   IdP, tenant and type as the exact directory lookup. Keep both raw
   CreateOCMIncomingShare RPCs, on Gateway and on OcmIncomingAPI, inside the
   trusted internal network. External offers go through the HTTP OCM policy.
3. Drain older SQL readers and writers before enabling exact bindings on the
   shared store. Check the provenance of any existing keyed rows with an empty
   IdP. On a serving canary, verify receipt and authenticated list, read and
   update with a fresh token after restarting against the same database.

```toml
[grpc.services.ocmincoming]
allow_verified_empty_recipient_idp = true
```

Leave the setting off until those checks pass. Requalify changes to the
directory, token issuer or listeners and retire stale sessions when the
identity source changes. Before switching versions, check active transfers.
For rollback, keep older builds away from the upgraded store. If restoring an
isolated compatible snapshot, account for writes made after that snapshot.
