
---
title: "v3.12.0"
linkTitle: "v3.12.0"
weight: 999580
description: >
  Changelog for Reva v3.12.0 (2026-07-30)
---

Changelog for reva 3.12.0 (2026-07-30)
=======================================

The following sections list the changes in reva 3.12.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5722: make /dav/files/<username>/<path> work again
 * Fix #5733: Fix nilptr exception in ListUsers
 * Fix #5698: Normalize user ids on incoming OCM shares
 * Fix #5695: Normalize remote user ids in OCM to avoid malformed shareWith
 * Fix #5696: Resolve single-file OCM shares correctly over WebDAV
 * Fix #5724: Pass token owner in public link scope check
 * Enh #5703: Apps: added temporary patch for Office apps
 * Enh #5737: EOS driver now takes modbits into account
 * Enh #5732: Implement feedback endpoint
 * Enh #5726: Use uppercase URL encoding to calculate signatures for signed urls
 * Enh #5705: Add endpoint for office mentions
 * Enh #5727: Add resource_admins column to the projects table
 * Enh #5708: Refactor notifications system
 * Enh #5699: Store OCM shares after the remote accepts them

Details
-------

 * Bugfix #5722: make /dav/files/<username>/<path> work again

   https://github.com/cs3org/reva/pull/5722

 * Bugfix #5733: Fix nilptr exception in ListUsers

   Specifcally, when the OrderBy parameter was omitted

   https://github.com/cs3org/reva/pull/5733

 * Bugfix #5698: Normalize user ids on incoming OCM shares

   This fixes cross-vendor OCM shares failing on the receiving side when Reva is the receiver and
   the peer is a non-conformant OCM server. It is the receiver-side counterpart to the
   sender-side normalization in #5695.

   The OCM spec expects an address as `<bare-id>@<provider>`, with the host kept separate from
   the user id. Some peers glue the host onto the id: oCIS sends `id@host` and OpenCloud sends
   `id@https://host`, and by the time it reaches the `POST /ocm/shares` handler the id is doubled
   up as `id@host@host` (with or without a scheme).

   `GetUserIdFromOCMAddress` only strips the final `@host`, so the parsed opaque id kept a
   leftover host suffix that broke user resolution, and the two vendors failed at two different
   points. For `shareWith` (the local recipient) the leftover host made the local user lookup
   miss and the request returned 404 user not found. For `sender` and `owner` (the remote sharer)
   it made the accepted-user lookup miss, because the accepted remote user is stored with a bare id
   during the invitation flow, and the request returned 401 unauthenticated.

   The incoming-share handler now parses these three addresses through a helper that also strips
   a redundant, self-referential provider suffix that matches the parsed provider domain.
   Identifiers that legitimately contain `@` for a different host are left untouched, and
   spec-conformant peers such as CERNBox-to-CERNBox and Nextcloud are unaffected.

   https://github.com/cs3org/reva/pull/5698

 * Bugfix #5695: Normalize remote user ids in OCM to avoid malformed shareWith

   This fixes cross-vendor OCM shares failing when Reva is the sender and the peer is a
   non-conformant OCM server. The OCM spec requires the invite `userID` to be the bare identifier
   of the user at their OCM Server, with the host carried separately in `recipientProvider`. Some
   peers instead send a fully-qualified `userID` such as `id@host` (oCIS) or `id@https://host`
   (OpenCloud).

   Reva stored that qualified string verbatim as the federated user's opaque id and later
   re-appended the provider domain when building `shareWith`, producing malformed recipients
   like `id@host@host` or `id@https://host@host`. Receivers could not resolve those to a local
   user, so the share silently never materialized even though the HTTP request returned 200 OK.

   Reva now normalizes remote user ids on ingress (invite acceptance and invite forwarding) by
   stripping a redundant, self-referential provider suffix that matches the known provider
   domain, and defensively formats outbound OCM Addresses so it never emits the `id@host@host`
   form. Identifiers that legitimately contain `@` for a different host are left untouched, and
   spec-conformant peers are unaffected.

   https://github.com/cs3org/reva/pull/5695

 * Bugfix #5696: Resolve single-file OCM shares correctly over WebDAV

   This fixes cross-vendor OCM shares failing to appear on the receiver when Reva is the sender and
   the shared resource is a single file. Remote receivers (oCIS, OpenCloud) mount an OCM share as a
   directory and address the shared file as `<share>/<name>`, so the incoming WebDAV request
   carries the file's own name as a relative path.

   The `ocmoutcoming` storage driver resolved the share token to the shared resource (the file
   itself) and then unconditionally joined the relative path onto it, producing a doubled path
   such as `/home/einstein/report.txt/report.txt`. That path does not exist, so the stat
   failed and the WebDAV `PROPFIND` returned HTTP 500. Receivers treat a failed stat as a missing
   share and silently drop it from their "shared with me" listing, so the file never became visible
   even though the share was created and accepted.

   The driver now resolves paths based on the shared resource type. Folder shares still nest the
   child beneath the container. For a single-file share, which has exactly one resource, both the
   share root and a single-segment child resolve to the file itself, so `<file>/<file>` is never
   built and receivers whose appended name differs from the storage path base are tolerated.
   Nested paths under a file share are rejected as malformed.

   https://github.com/cs3org/reva/pull/5696

 * Bugfix #5724: Pass token owner in public link scope check

   https://github.com/cs3org/reva/pull/5724

 * Enhancement #5703: Apps: added temporary patch for Office apps

   This PR adds the handling of language and theme input parameters, and returns an
   app_for_editing additional parameter in response if appropriate.

   https://github.com/cs3org/reva/pull/5703

 * Enhancement #5737: EOS driver now takes modbits into account

   To make /app/open also take into account modbits (so you can open files in apps in the public
   folder)

   https://github.com/cs3org/reva/pull/5737

 * Enhancement #5732: Implement feedback endpoint

   Implement an endpoint so users can send feedback, to be used for the CERNBox Office Pilot.

   https://github.com/cs3org/reva/pull/5732

 * Enhancement #5726: Use uppercase URL encoding to calculate signatures for signed urls

   https://github.com/cs3org/reva/pull/5726

 * Enhancement #5705: Add endpoint for office mentions

   This change adds a POST /app/mentions endpoint for Office integrations to trigger mention
   notifications.

   https://github.com/cs3org/reva/pull/5705

 * Enhancement #5727: Add resource_admins column to the projects table

   The projects table now has a resource_admins column, holding the e-group ID of the resource
   administrators of a project.

   https://github.com/cs3org/reva/pull/5727

 * Enhancement #5708: Refactor notifications system

   Following ADR general/0007-notifications-refactor

   We now use an event-based system, where handlers register to events, and a configurable
   rule-system determines how to handle these. Accumulation is coordinated between daemons.

   https://github.com/cs3org/reva/pull/5708

 * Enhancement #5699: Store OCM shares after the remote accepts them

   This commit introduces a behaviour where the OCM shares are only stored locally if the remote
   system accepts them first, meaning we don't have any orphaned OCM shares locally

   https://github.com/cs3org/reva/pull/5699


