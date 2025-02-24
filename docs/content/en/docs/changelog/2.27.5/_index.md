
---
title: "v2.27.5"
linkTitle: "v2.27.5"
weight: 40
description: >
  Changelog for Reva v2.27.5 (2025-02-24)
---

Changelog for reva 2.27.5 (2025-02-24)
=======================================

The following sections list the changes in reva 2.27.5 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #5093: Fix OCM create share
*   Fix #5077: Deny Users invite themselves to their own federated connection
*   Fix #5071: Role conversion
*   Enh #5075: Add the ocm notification handler
*   Enh #5083: Add the ocm notification ShareChangePermission
*   Enh #5063: Add roles

Details
-------

*   Bugfix #5093: Fix OCM create share

   We fixed the OCM share fails on share creating if the federated instance is not reachable.

   https://github.com/owncloud/ocis/issues/11046
   https://github.com/cs3org/reva/pull/5093

*   Bugfix #5077: Deny Users invite themselves to their own federated connection

   Deny Users invite themselves to their own federated connection

   https://github.com/cs3org/reva/pull/5077

*   Bugfix #5071: Role conversion

   Fix role from resource permission conversion

   https://github.com/cs3org/reva/pull/5071

*   Enhancement #5075: Add the ocm notification handler

   Added the ocm notification handler that allows receiving a notification from a remote party
   about changes to a previously known entity.

   https://github.com/cs3org/reva/pull/5075

*   Enhancement #5083: Add the ocm notification ShareChangePermission

   Added the ocm notification ShareChangePermission that allows to synchronize the
   permissions of a share between the federated instances.

   https://github.com/cs3org/reva/pull/5083

*   Enhancement #5063: Add roles

   Add EditorListGrantsWithVersions and FileEditorListGrantsWithVersions roles.

   https://github.com/owncloud/ocis/issues/10747
   https://github.com/cs3org/reva/pull/5063

