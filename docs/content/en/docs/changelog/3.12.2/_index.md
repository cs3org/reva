
---
title: "v3.12.2"
linkTitle: "v3.12.2"
weight: 999578
description: >
  Changelog for Reva v3.12.2 (2026-08-14)
---

Changelog for reva 3.12.2 (2026-08-14)
=======================================

The following sections list the changes in reva 3.12.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Sec #5752: Harden OCM federation against SSRF, spoofed IPs and TLS downgrade
 * Fix #5761: Report the recursive size of directories in cephmount
 * Fix #5766: Skip fetching group members when creating a share
 * Enh #5740: Cephmount now supports shares to external accounts
 * Enh #5745: Ignore Range requests on empty files
 * Enh #5764: Return lock info in propfind
 * Enh #5769: Keep timestamp instead of bool for share orphan
 * Enh #5736: Enable quota cache for projects
 * Enh #5738: Return file name on PUT to upload-only public link
 * Enh #5762: Move share permission checks to the gateway

Details
-------

 * Security #5752: Harden OCM federation against SSRF, spoofed IPs and TLS downgrade

   The unauthenticated ScienceMesh `/discover` endpoint no longer connects to loopback,
   private, link-local or cloud-metadata addresses. The OCM provider allow list no longer
   trusts a caller-supplied `X-Forwarded-For` header, and remote provider discovery now
   verifies TLS certificates by default.

   https://github.com/cs3org/reva/pull/5752

 * Bugfix #5761: Report the recursive size of directories in cephmount

   Directories were reported with a size of 0, so a folder's size did not match the contents seen
   inside it. The size of the subtree is now taken from the CephFS `ceph.dir.rbytes` extended
   attribute.

   https://github.com/cs3org/reva/pull/5761

 * Bugfix #5766: Skip fetching group members when creating a share

   https://github.com/cs3org/reva/pull/5766

 * Enhancement #5740: Cephmount now supports shares to external accounts

   * Grants to external accounts are now stored in a `user.reva.extshare.<account>` xattr and
   are read back by ListGrants. * Operations on their behalf run as the service account configured
   with `external_accounts_user_name`/`_uid`/`_gid`, cboxexternal by default, the same
   account the EOS driver uses. * What they are allowed to do is decided by Reva from the stored grant
   rather than by the permissions of that account. * The ResourceInfo of a shared resource now also
   carries a name and an etag which now is recursive, meaning you changes in subtrees also get
   propgated.

   https://github.com/cs3org/reva/pull/5740

 * Enhancement #5745: Ignore Range requests on empty files

   https://github.com/cs3org/reva/pull/5745

 * Enhancement #5764: Return lock info in propfind

   https://github.com/cs3org/reva/pull/5764

 * Enhancement #5769: Keep timestamp instead of bool for share orphan

   https://github.com/cs3org/reva/pull/5769

 * Enhancement #5736: Enable quota cache for projects

   https://github.com/cs3org/reva/pull/5736

 * Enhancement #5738: Return file name on PUT to upload-only public link

   https://github.com/cs3org/reva/pull/5738

 * Enhancement #5762: Move share permission checks to the gateway

   https://github.com/cs3org/reva/pull/5762


