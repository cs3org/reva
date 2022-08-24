
---
title: "v2.8.0"
linkTitle: "v2.8.0"
weight: 40
description: >
  Changelog for Reva v2.8.0 (2022-08-23)
---

Changelog for reva 2.8.0 (2022-08-23)
=======================================

The following sections list the changes in reva 2.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3158: Add name to the propfind response
*   Fix #3157: Fix locking response codes
*   Fix #3152: Disable caching of not found stat responses
*   Fix #4251: Disable caching
*   Chg #3154: Dataproviders now return file metadata
*   Enh #3143: Add /app/open-with-web endpoint
*   Enh #3156: Added language option to the app provider
*   Enh #3148: Add new jsoncs3 share manager

Details
-------

*   Bugfix #3158: Add name to the propfind response

   Previously the file- or foldername had to be extracted from the href. This is not nice and
   doesn't work for alias links.

   https://github.com/cs3org/reva/pull/3158

*   Bugfix #3157: Fix locking response codes

   We've fixed the status codes for locking a file that is already locked.

   https://github.com/owncloud/ocis/issues/4366
   https://github.com/cs3org/reva/pull/3157
   https://github.com/cs3org/reva/pull/3003

*   Bugfix #3152: Disable caching of not found stat responses

   We no longer cache not found responses to prevent concurrent requests interfering with put
   requests.

   https://github.com/owncloud/ocis/issues/4251
   https://github.com/cs3org/reva/pull/3152

*   Bugfix #4251: Disable caching

   We disabled the cache, because there are race condtions that cause tests to fail.

   https://github.com/owncloud/ocis/issues/4251
   https://github.com/cs3org/reva/pull/3167

*   Change #3154: Dataproviders now return file metadata

   Dataprovider drivers can now return file metadata. When the resource info contains a file id,
   the mtime or an etag, these will be included in the response as the corresponding http headers.

   https://github.com/cs3org/reva/pull/3154

*   Enhancement #3143: Add /app/open-with-web endpoint

   We've added an /app/open-with-web endpoint to the app provider, so that clients that are no
   browser or have only limited browser access can also open apps with the help of a Web URL.

   https://github.com/cs3org/reva/pull/3143
   https://github.com/owncloud/ocis/pull/4376

*   Enhancement #3156: Added language option to the app provider

   We've added an language option to the app provider which will in the end be passed to the app a user
   opens so that the web ui is displayed in the users language.

   https://github.com/owncloud/ocis/issues/4367
   https://github.com/cs3org/reva/pull/3156
   https://github.com/owncloud/ocis/pull/4399

*   Enhancement #3148: Add new jsoncs3 share manager

   We've added a new jsoncs3 share manager which splits the json file per storage space and caches
   data locally.

   https://github.com/cs3org/reva/pull/3148

