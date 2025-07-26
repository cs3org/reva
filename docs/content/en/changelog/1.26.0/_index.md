
---
title: "v1.26.0"
linkTitle: "v1.26.0"
weight: 40
description: >
  Changelog for Reva v1.26.0 (2023-09-08)
---

Changelog for reva 1.26.0 (2023-09-08)
=======================================

The following sections list the changes in reva 1.26.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4165: Use default user tmp folder in config tests
 * Fix #4113: Fix plugin's registration when reva is built with version 1.21
 * Fix #4171: Fix accessing an OCM-shared resource containing spaces
 * Fix #4172: Hardcode access methods for outgoing OCM shares from OC/NC
 * Fix #4125: Enable projects for lightweight accounts
 * Enh #4121: Expire cached users and groups entries
 * Enh #4162: Disable sharing on a storage provider
 * Enh #4163: Disable trashbin on a storage provider
 * Enh #4164: Disable versions on a storage provider
 * Enh #4084: Implementation of an app provider for Overleaf
 * Enh #4114: List all the registered plugins
 * Enh #4115: All required features and fixes for the OC/NC ScienceMesh apps

Details
-------

 * Bugfix #4165: Use default user tmp folder in config tests

   https://github.com/cs3org/reva/pull/4165

 * Bugfix #4113: Fix plugin's registration when reva is built with version 1.21

   With go 1.21 the logic for package initialization has changed, and the plugins were failing in
   the registration. Now the registration of the plugins is deferred in the main.

   https://github.com/cs3org/reva/pull/4113

 * Bugfix #4171: Fix accessing an OCM-shared resource containing spaces

   Fixes the access of a resource OCM-shared containing spaces, that previously was failing with
   a `NotFound` error.

   https://github.com/cs3org/reva/pull/4171

 * Bugfix #4172: Hardcode access methods for outgoing OCM shares from OC/NC

   This is a workaround until sciencemesh/nc-sciencemesh#45 is properly implemented

   https://github.com/cs3org/reva/pull/4172

 * Bugfix #4125: Enable projects for lightweight accounts

   Enable CERNBox projects to be listed by a lightweight account

   https://github.com/cs3org/reva/pull/4125

 * Enhancement #4121: Expire cached users and groups entries

   Entries in the rest user and group drivers do not expire. This means that old users/groups that
   have been deleted are still in cache. Now an expiration of `fetch interval + 1` hours has been
   set.

   https://github.com/cs3org/reva/pull/4121

 * Enhancement #4162: Disable sharing on a storage provider

   Added a GRPC interceptor that disable sharing permissions on a storage provider.

   https://github.com/cs3org/reva/pull/4162

 * Enhancement #4163: Disable trashbin on a storage provider

   Added a GRPC interceptor that disable the trashbin on a storage provider.

   https://github.com/cs3org/reva/pull/4163

 * Enhancement #4164: Disable versions on a storage provider

   Added a GRPC interceptor that disable the versions on a storage provider.

   https://github.com/cs3org/reva/pull/4164

 * Enhancement #4084: Implementation of an app provider for Overleaf

   This PR adds an app provider for Overleaf as a standalone http service.

   The app provider currently consists of support for the export to Overleaf feature, which when
   called returns a URL to Overleaf that prompts Overleaf to download the appropriate resource
   making use of the Archiver service, and upload the files to a user's Overleaf account.

   https://github.com/cs3org/reva/pull/4084

 * Enhancement #4114: List all the registered plugins

   https://github.com/cs3org/reva/pull/4114

 * Enhancement #4115: All required features and fixes for the OC/NC ScienceMesh apps

   This PR includes all necessary code in Reva to interface with the ScienceMesh apps in OC and NC

   https://github.com/cs3org/reva/pull/4115


