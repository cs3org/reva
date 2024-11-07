
---
title: "v2.26.4"
linkTitle: "v2.26.4"
weight: 40
description: >
  Changelog for Reva v2.26.4 (2024-11-07)
---

Changelog for reva 2.26.4 (2024-11-07)
=======================================

The following sections list the changes in reva 2.26.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4917: Fix 0-byte file uploads
*   Fix #4918: Fix app templates

Details
-------

*   Bugfix #4917: Fix 0-byte file uploads

   We fixed an issue where 0-byte files upload did not return the Location header.

   https://github.com/owncloud/ocis/issues/10469
   https://github.com/cs3org/reva/pull/4917

*   Bugfix #4918: Fix app templates

   We fixed the app templates by using the product name of the providerinfo instead of the provider
   name.

   https://github.com/cs3org/reva/pull/4918

