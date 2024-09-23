
---
title: "v2.19.8"
linkTitle: "v2.19.8"
weight: 40
description: >
  Changelog for Reva v2.19.8 (2024-09-23)
---

Changelog for reva 2.19.8 (2024-09-23)
=======================================

The following sections list the changes in reva 2.19.8 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4761: Quotes in dav Content-Disposition header
*   Fix #4853: Write upload session info atomically
*   Enh #4701: Extend service account permissions

Details
-------

*   Bugfix #4761: Quotes in dav Content-Disposition header

   We've fixed the the quotes in the dav `Content-Disposition` header. They caused an issue where
   certain browsers would decode the quotes and falsely prepend them to the filename.

   https://github.com/owncloud/web/issues/11031
   https://github.com/owncloud/web/issues/11169
   https://github.com/cs3org/reva/pull/4761

*   Bugfix #4853: Write upload session info atomically

   We now use a lock and atomic write on upload session metadata to prevent empty reads. A virus scan
   event might cause the file to be truncated and then a finished event might try to read the file,
   just getting an empty string.

   Backport of https://github.com/cs3org/reva/pull/4850

   https://github.com/cs3org/reva/pull/4853

*   Enhancement #4701: Extend service account permissions

   Adds AddGrant permisson

   https://github.com/cs3org/reva/pull/4701

