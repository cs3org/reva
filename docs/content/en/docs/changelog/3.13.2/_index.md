
---
title: "v3.13.2"
linkTitle: "v3.13.2"
weight: 999568
description: >
  Changelog for Reva v3.13.2 (2026-10-14)
---

Changelog for reva 3.13.2 (2026-10-14)
=======================================

The following sections list the changes in reva 3.13.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5862: Wrong checksum type and value reported on EOS
 * Fix #5899: Fix issue with files with % in name
 * Fix #5897: Fall back to the sharer as OCM share owner
 * Fix #5882: Normal users were incorrectly resolved as ocm users
 * Fix #5881: Stop blaming nodes for their backends
 * Fix #5893: Don't allow sibling directory copy
 * Enh #5896: Allow lightweight accounts to share
 * Enh #5883: Configurable registry selector

Details
-------

 * Bugfix #5862: Wrong checksum type and value reported on EOS

   With the gRPC client the checksum type came out as INVALID and the value carried 56 zeros of
   padding, so a client comparing checksums never matched.

   https://github.com/cs3org/reva/pull/5862

 * Bugfix #5899: Fix issue with files with % in name

   https://github.com/cs3org/reva/pull/5899

 * Bugfix #5897: Fall back to the sharer as OCM share owner

   When the resource owner can't be resolved to a user, as for files in lightweight homes, the OCM
   share was sent with an empty owner address and rejected by the remote server. The sharer is now
   used as the owner instead.

   https://github.com/cs3org/reva/pull/5897

 * Bugfix #5882: Normal users were incorrectly resolved as ocm users

   When listing OCM shares the grantee was incorrectly looked up as an ocm user but in fact it will
   always be a local user.

   https://github.com/cs3org/reva/pull/5882

 * Bugfix #5881: Stop blaming nodes for their backends

   A node relaying an Unavailable is no longer penalized, draining nodes no longer hide penalized
   ones, and an unverifiable token is answered with Unavailable. Node selection and failover
   decisions are logged at trace level.

   https://github.com/cs3org/reva/pull/5881

 * Bugfix #5893: Don't allow sibling directory copy

   Don't allow sibling directory copy on writeable links

   https://github.com/cs3org/reva/pull/5893/

 * Enhancement #5896: Allow lightweight accounts to share

   The lightweight scope now allows searching users and creating user, group and OCM shares, so
   lightweight accounts can share from the graph API.

   https://github.com/cs3org/reva/pull/5896

 * Enhancement #5883: Configurable registry selector

   `selector` in `[shared.registry]` chooses local (the default: a node on the same host, else a
   random one), first, random or roundrobin, and a service's nodes are listed in id order.

   https://github.com/cs3org/reva/pull/5883


