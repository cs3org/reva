
---
title: "v2.26.3"
linkTitle: "v2.26.3"
weight: 40
description: >
  Changelog for Reva v2.26.3 (2024-11-06)
---

Changelog for reva 2.26.3 (2024-11-06)
=======================================

The following sections list the changes in reva 2.26.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4908: Add checksum to OCM storageprovider responses
*   Enh #4910: Bump cs3api
*   Enh #4909: Bump cs3api
*   Enh #4906: Bump cs3api

Details
-------

*   Bugfix #4908: Add checksum to OCM storageprovider responses

   When the remote instance of the OCM storage provider returns file checksums in its PROPFIND
   responses we're now passing them through to in Stat responses. This allows e.g. the oCIS
   thumbnailer to work with ocm shares.

   https://github.com/owncloud/ocis/issues/10272
   https://github.com/cs3org/reva/pull/4908

*   Enhancement #4910: Bump cs3api

   https://github.com/cs3org/reva/pull/4910

*   Enhancement #4909: Bump cs3api

   https://github.com/cs3org/reva/pull/4909

*   Enhancement #4906: Bump cs3api

   https://github.com/cs3org/reva/pull/4906

