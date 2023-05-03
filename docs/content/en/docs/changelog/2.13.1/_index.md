
---
title: "v2.13.1"
linkTitle: "v2.13.1"
weight: 40
description: >
  Changelog for Reva v2.13.1 (2023-05-03)
---

Changelog for reva 2.13.1 (2023-05-03)
=======================================

The following sections list the changes in reva 2.13.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3843: Allow scope check to impersonate space owners

Details
-------

*   Bugfix #3843: Allow scope check to impersonate space owners

   The publicshare scope check now fakes a user to mint an access token when impersonating a user of
   type `SPACE_OWNER` which is used for project spaces. This fixes downloading archives from
   public link shares in project spaces.

   https://github.com/owncloud/ocis/issues/5229
   https://github.com/cs3org/reva/pull/3843

