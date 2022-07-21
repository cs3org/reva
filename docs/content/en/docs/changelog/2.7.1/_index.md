
---
title: "v2.7.1"
linkTitle: "v2.7.1"
weight: 40
description: >
  Changelog for Reva v2.7.1 (2022-07-15)
---

Changelog for reva 2.7.1 (2022-07-15)
=======================================

The following sections list the changes in reva 2.7.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3080: Make dataproviders return more headers
 * Enh #4072: Add user filter

Details
-------

 * Bugfix #3080: Make dataproviders return more headers

   Instead of ocdav doing an additional Stat request we now rely on the dataprovider to return the
   necessary metadata information as headers.

   https://github.com/owncloud/reva/issues/3080

 * Enhancement #3046: Add user filter

   This PR adds the ability to filter spaces by user-id

   https://github.com/cs3org/reva/pull/3046


