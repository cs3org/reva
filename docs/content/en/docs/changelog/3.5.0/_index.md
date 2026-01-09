
---
title: "v3.5.0"
linkTitle: "v3.5.0"
weight: 999650
description: >
  Changelog for Reva v3.5.0 (2026-01-09)
---

Changelog for reva 3.5.0 (2026-01-09)
=======================================

The following sections list the changes in reva 3.5.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5449: Fix database sharedconfig parsing
 * Fix #5464: Make MOVE work in public link
 * Fix #5446: Make notify_uploads_extra_recipients work in libregraph
 * Fix #5463: Fix permission editing for public links
 * Fix #5404: Fix localhome virtual namespace path handling for spaces
 * Enh #5429: Cephmount: supports setting posix acls
 * Enh #5433: Make chunking_parallel_upload_disabled configurable
 * Enh #5450: Clean up ListWithRegex log
 * Enh #5447: Make notification trigger always use `items`
 * Enh #5445: Support funcs in templates
 * Enh #5402: Refactoring of the GORM model for shares
 * Enh #5428: Refactor permissions

Details
-------

 * Bugfix #5449: Fix database sharedconfig parsing

   https://github.com/cs3org/reva/pull/5449

 * Bugfix #5464: Make MOVE work in public link

   https://github.com/cs3org/reva/pull/5464

 * Bugfix #5446: Make notify_uploads_extra_recipients work in libregraph

   https://github.com/cs3org/reva/pull/5446

 * Bugfix #5463: Fix permission editing for public links

   A bug was introduced during the refactoring of the permission system
   (https://github.com/cs3org/reva/pull/5428). This has now been fixed.

   https://github.com/cs3org/reva/pull/5463

 * Bugfix #5404: Fix localhome virtual namespace path handling for spaces

   Added optional VirtualHomeTemplate config to localfs driver, enabling localhome to
   correctly handle paths when exposing user homes through a virtual namespace (e.g.,
   /home/<user>) while storing files in a flat per-user layout on disk.

   The wrap() function uses a clean switch statement with named predicates to handle five path
   transformation patterns:

   - Exact match: /home/einstein -> / - Full path: /home/einstein/file -> /file - Parent path:
   /home -> / (when virtual home is /home/einstein) - Gateway-stripped parent: /home/file ->
   /file (gateway omits username) - Gateway-stripped username: /einstein/file -> /file
   (WebDAV "home" alias)

   The last two cases handle gateway routing edge cases where prefixes are stripped differently
   depending on whether the WebDAV layer uses space IDs or the "home" alias for URL construction.

   The normalize() function adds the virtual home prefix only to the Path field of ResourceInfo
   (e.g., /file -> /home/einstein/file), enabling PathToSpaceID() to derive the correct space
   identifier. The OpaqueId field remains storage-relative (e.g., fileid-einstein%2Ffile)
   to ensure resource IDs can be properly decoded.

   The localhome wrapper now correctly passes VirtualHomeTemplate through to localfs.

   When VirtualHomeTemplate is empty (default), behavior is unchanged, ensuring backward
   compatibility with EOS and existing deployments.

   https://github.com/cs3org/reva/pull/5404

 * Enhancement #5429: Cephmount: supports setting posix acls

   https://github.com/cs3org/reva/pull/5429

 * Enhancement #5433: Make chunking_parallel_upload_disabled configurable

   https://github.com/cs3org/reva/pull/5433

 * Enhancement #5450: Clean up ListWithRegex log

   https://github.com/cs3org/reva/pull/5450

 * Enhancement #5447: Make notification trigger always use `items`

   https://github.com/cs3org/reva/pull/5447

 * Enhancement #5445: Support funcs in templates

   https://github.com/cs3org/reva/pull/5445

 * Enhancement #5402: Refactoring of the GORM model for shares

   With this PR we introduce new constraints and rename some fields for better consistency:

   * Types used by OCM structures only are prefixed with `Ocm`, and `AccessMethod` and `Protocol`
   were consolidated into `OcmProtocol` * ItemType is used in OCM shares as well * The
   `(FileIdPrefix, ItemSource)` tuple is now `(Instance, Inode)` in `OcmShare`, and it was
   removed from `OcmReceivedShare` as unused * Unique index constraints have been created for
   regular `Shares` and for `OcmShares` on `(instance, inode, shareWith, deletedAt)` * The
   unique indexes have been renamed with a `u_` prefix for consistency: this affected
   `u_shareid_user`, `u_link_token`. The `i_share_with` was dropped as redundant. * `Alias`
   and `Hidden` were added in `OcmReceivedShare`

   https://github.com/cs3org/reva/pull/5402

 * Enhancement #5428: Refactor permissions

   Permissions are now, at least partially, handled and exposed within a single package (which
   was important for cernboxcop), with conversions between the different types of permissions

   https://github.com/cs3org/reva/pull/5428


