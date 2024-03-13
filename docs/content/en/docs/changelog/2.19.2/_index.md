
---
title: "v2.19.2"
linkTitle: "v2.19.2"
weight: 40
description: >
  Changelog for Reva v2.19.2 (2024-03-13)
---

Changelog for reva 2.19.2 (2024-03-13)
=======================================

The following sections list the changes in reva 2.19.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4557: Fix ceph build
*   Fix #4570: Fix sharing invite on virtual drive
*   Fix #4559: Fix graph drive invite
*   Fix #4518: Fix an error when lock/unlock a file
*   Fix #4566: Fix public link previews
*   Fix #4561: Fix Stat() by Path on re-created resource
*   Enh #4556: Allow tracing requests by giving util functions a context
*   Enh #4545: Extend service account permissions
*   Enh #4564: Send file locked/unlocked events

Details
-------

*   Bugfix #4557: Fix ceph build

   https://github.com/cs3org/reva/pull/4557

*   Bugfix #4570: Fix sharing invite on virtual drive

   We fixed the issue when sharing of virtual drive with other users was allowed

   https://github.com/owncloud/ocis/issues/8495
   https://github.com/cs3org/reva/pull/4570

*   Bugfix #4559: Fix graph drive invite

   We fixed the issue when sharing of personal drive is allowed via graph

   https://github.com/owncloud/ocis/issues/8494
   https://github.com/cs3org/reva/pull/4559

*   Bugfix #4518: Fix an error when lock/unlock a file

   We fixed a bug when anonymous user with viewer role in public link of a folder can lock/unlock a
   file inside it

   https://github.com/owncloud/ocis/issues/7785
   https://github.com/cs3org/reva/pull/4518

*   Bugfix #4566: Fix public link previews

   Fixes previews for public links

   https://github.com/cs3org/reva/pull/4566

*   Bugfix #4561: Fix Stat() by Path on re-created resource

   We fixed bug that caused Stat Requests using a Path reference to a mount point in the sharejail to
   not resolve correctly, when a share using the same mount point to an already deleted resource
   was still existing.

   https://github.com/owncloud/ocis/issues/7895
   https://github.com/cs3org/reva/pull/4561

*   Enhancement #4556: Allow tracing requests by giving util functions a context

   We deprecated GetServiceUserContext with GetServiceUserContextWithContext and GetUser
   with GetUserWithContext to allow passing in a trace context.

   https://github.com/cs3org/reva/pull/4556

*   Enhancement #4545: Extend service account permissions

   Adds CreateContainer permisson and improves cs3 storage pkg

   https://github.com/cs3org/reva/pull/4545

*   Enhancement #4564: Send file locked/unlocked events

   Emit an event when a file is locked or unlocked

   https://github.com/cs3org/reva/pull/4564

