
---
title: "v1.10.0"
linkTitle: "v1.10.0"
weight: 40
description: >
  Changelog for Reva v1.10.0 (2021-07-13)
---

Changelog for reva 1.10.0 (2021-07-13)
=======================================

The following sections list the changes in reva 1.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1883: Pass directories with trailing slashes to eosclient.GenerateToken
 * Fix #1878: Improve the webdav error handling in the trashbin
 * Fix #1884: Do not send body on failed range request
 * Enh #1744: Add support for lightweight user types

Details
-------

 * Bugfix #1883: Pass directories with trailing slashes to eosclient.GenerateToken

   https://github.com/cs3org/reva/pull/1883

 * Bugfix #1878: Improve the webdav error handling in the trashbin

   The trashbin handles errors better now on the webdav endpoint.

   https://github.com/cs3org/reva/pull/1878

 * Bugfix #1884: Do not send body on failed range request

   Instead of send the error in the body of a 416 response we log it. This prevents the go reverse
   proxy from choking on it and turning it into a 502 Bad Gateway response.

   https://github.com/cs3org/reva/pull/1884

 * Enhancement #1744: Add support for lightweight user types

   This PR adds support for assigning and consuming user type when setting/reading users. On top
   of that, support for lightweight users is added. These users have to be restricted to accessing
   only shares received by them, which is accomplished by expanding the existing RBAC scope.

   https://github.com/cs3org/reva/pull/1744
   https://github.com/cs3org/cs3apis/pull/120


