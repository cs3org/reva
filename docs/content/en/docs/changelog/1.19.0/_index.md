
---
title: "v1.19.0"
linkTitle: "v1.19.0"
weight: 40
description: >
  Changelog for Reva v1.19.0 (2022-06-16)
---

Changelog for reva 1.19.0 (2022-06-16)
=======================================

The following sections list the changes in reva 1.19.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2693: Support editnew actions from MS Office
 * Fix #2588: Dockerfile.revad-ceph to use the right base image
 * Fix #2216: Make hardcoded HTTP "insecure" options configurable
 * Fix #2860: Use `eos-all` parent image
 * Fix #2499: Removed check DenyGrant in resource permission
 * Fix #2712: Update Dockerfile.revad.eos to not break the image
 * Fix #2789: Minor fixes in cephfs and eosfs
 * Fix #2285: Accept new userid idp format
 * Fix #2608: Respect the tracing_service_name config variable
 * Fix #2841: Refactors logger to have ctx
 * Fix #2759: Made uid, gid claims parsing more robust in OIDC auth provider
 * Fix #2842: Fix download action in SDK
 * Fix #2555: Fix site accounts endpoints
 * Fix #2675: Updates Makefile according to latest go standards
 * Fix #2572: Wait for nats server on middleware start
 * Chg #2596: Remove hash from public link urls
 * Chg #2559: Do not encode webDAV ids to base64
 * Chg #2561: Merge oidcmapping auth manager into oidc
 * Enh #2698: Make capabilities endpoint public, authenticate users is present
 * Enh #2813: Support custom mimetypes in the WOPI appprovider driver
 * Enh #2515: Enabling tracing by default if not explicitly disabled
 * Enh #160: Implement the CS3 Lock API in the EOS storage driver
 * Enh #2686: Features for favorites xattrs in EOS, cache for scope expansion
 * Enh #2494: Use sys ACLs for file permissions
 * Enh #2522: Introduce events
 * Enh #2685: Enable federated account access
 * Enh #2801: Use functional options for client gRPC connections
 * Enh #2921: Use standard header for checksums
 * Enh #2480: Group based capabilities
 * Enh #1787: Add support for HTTP TPC
 * Enh #2560: Mentix PromSD extensions
 * Enh #2613: Externalize custom mime types configuration for storage providers
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2696: Preferences driver refactor and cbox sql implementation
 * Enh #2052: New CS3API datatx methods
 * Enh #2738: Site accounts site-global settings
 * Enh #2672: Further Site Accounts improvements
 * Enh #2549: Site accounts improvements
 * Enh #2488: Cephfs support keyrings with IDs
 * Enh #2514: Reuse ocs role objects in other drivers
 * Enh #2752: Refactor the rest user and group provider drivers
 * Enh #2946: Make user share indicators read from the share provider service

Details
-------

 * Bugfix #2693: Support editnew actions from MS Office

   This fixes the incorrect behavior when creating new xlsx and pptx files, as MS Office supports
   the editnew action and it must be used for newly created files instead of the normal edit action.

   https://github.com/cs3org/reva/pull/2693

 * Bugfix #2588: Dockerfile.revad-ceph to use the right base image

   In Aug2021 https://hub.docker.com/r/ceph/daemon-base was moved to quay.ceph.io and the
   builds for this image were failing for some weeks after January.

   https://github.com/cs3org/reva/pull/2588

 * Bugfix #2216: Make hardcoded HTTP "insecure" options configurable

   HTTP "insecure" options must be configurable and default to false.

   https://github.com/cs3org/reva/issues/2216

 * Bugfix #2860: Use `eos-all` parent image

   https://github.com/cs3org/reva/pull/2860

 * Bugfix #2499: Removed check DenyGrant in resource permission

   When adding a denial permission

   https://github.com/cs3org/reva/pull/2499

 * Bugfix #2712: Update Dockerfile.revad.eos to not break the image

   https://github.com/cs3org/reva/pull/2712

 * Bugfix #2789: Minor fixes in cephfs and eosfs

   https://github.com/cs3org/reva/pull/2789

 * Bugfix #2285: Accept new userid idp format

   The format for userid idp [changed](https://github.com/cs3org/cs3apis/pull/159) and
   this broke [the ocmd
   tutorial](https://reva.link/docs/tutorials/share-tutorial/#5-1-4-create-the-share)
   This PR makes the provider authorizer interceptor accept both the old and the new string
   format.

   https://github.com/cs3org/reva/issues/2285
   https://github.com/cs3org/reva/issues/2285
   See
   and

 * Bugfix #2608: Respect the tracing_service_name config variable

   https://github.com/cs3org/reva/pull/2608

 * Bugfix #2841: Refactors logger to have ctx

   This fixes the native library loggers which are not associated with the context and thus are not
   handled properly in the reva runtime.

   https://github.com/cs3org/reva/pull/2841

 * Bugfix #2759: Made uid, gid claims parsing more robust in OIDC auth provider

   This fix makes sure the uid and gid claims are defined at init time, and that the necessary
   typecasts are performed correctly when authenticating users. A comment was added that in case
   the uid/gid claims are missing AND that no mapping takes place, a user entity is returned with
   uid = gid = 0.

   https://github.com/cs3org/reva/pull/2759

 * Bugfix #2842: Fix download action in SDK

   The download action was no longer working in the SDK (used by our testing probes); this PR fixes
   the underlying issue.

   https://github.com/cs3org/reva/pull/2842

 * Bugfix #2555: Fix site accounts endpoints

   This PR fixes small bugs in the site accounts endpoints.

   https://github.com/cs3org/reva/pull/2555

 * Bugfix #2675: Updates Makefile according to latest go standards

   Earlier, we were using go get to install packages. Now, we are using go install to install
   packages

   https://github.com/cs3org/reva/issues/2675
   https://github.com/cs3org/reva/pull/2747

 * Bugfix #2572: Wait for nats server on middleware start

   Use a retry mechanism to connect to the nats server when it is not ready yet

   https://github.com/cs3org/reva/pull/2572

 * Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

 * Change #2559: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2559

 * Change #2561: Merge oidcmapping auth manager into oidc

   The oidcmapping auth manager was created as a separate package to ease testing. As it has now
   been tested also as a pure OIDC auth provider without mapping, and as the code is largely
   refactored, it makes sense to merge it back so to maintain a single OIDC manager.

   https://github.com/cs3org/reva/pull/2561

 * Enhancement #2698: Make capabilities endpoint public, authenticate users is present

   https://github.com/cs3org/reva/pull/2698

 * Enhancement #2813: Support custom mimetypes in the WOPI appprovider driver

   Similarly to the storage provider, also the WOPI appprovider driver now supports custom mime
   types. Also fixed a small typo.

   https://github.com/cs3org/reva/pull/2813

 * Enhancement #2515: Enabling tracing by default if not explicitly disabled

   https://github.com/cs3org/reva/pull/2515

 * Enhancement #160: Implement the CS3 Lock API in the EOS storage driver

   https://github.com/cs3org/cs3apis/pull/160
   https://github.com/cs3org/reva/pull/2444

 * Enhancement #2686: Features for favorites xattrs in EOS, cache for scope expansion

   https://github.com/cs3org/reva/pull/2686

 * Enhancement #2494: Use sys ACLs for file permissions

   https://github.com/cs3org/reva/pull/2494

 * Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

 * Enhancement #2685: Enable federated account access

   https://github.com/cs3org/reva/pull/2685

 * Enhancement #2801: Use functional options for client gRPC connections

   This will add more ability to configure the client side gRPC connections.

   https://github.com/cs3org/reva/pull/2801

 * Enhancement #2921: Use standard header for checksums

   On HEAD requests, we currently expose checksums (when available) using the
   ownCloud-specific header, which is typically consumed by the sync clients.

   This patch adds the standard Digest header using the standard format detailed at
   https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Digest. This is e.g. used
   by GFAL/Rucio clients in the context of managed transfers of datasets.

   https://github.com/cs3org/reva/pull/2921

 * Enhancement #2480: Group based capabilities

   We can now return specific capabilities for users who belong to certain configured groups.

   https://github.com/cs3org/reva/pull/2480

 * Enhancement #1787: Add support for HTTP TPC

   We have added support for HTTP Third Party Copy. This allows remote data transfers between
   storages managed by either two different reva servers, or a reva server and a Grid
   (WLCG/ESCAPE) site server.

   Such remote transfers are expected to be driven by
   [GFAL](https://cern.ch/dmc-docs/gfal2/gfal2.html), the underlying library used by
   [FTS](https://cern.ch/fts), and [Rucio](https://rucio.cern.ch).

   In addition, the oidcmapping package has been refactored to support the standard OIDC use
   cases as well when no mapping is defined.

   https://github.com/cs3org/reva/issues/1787
   https://github.com/cs3org/reva/pull/2007

 * Enhancement #2560: Mentix PromSD extensions

   The Mentix Prometheus SD scrape targets are now split into one file per service type, making
   health checks configuration easier. Furthermore, the local file connector for mesh data and
   the site registration endpoint have been dropped, as they aren't needed anymore.

   https://github.com/cs3org/reva/pull/2560

 * Enhancement #2613: Externalize custom mime types configuration for storage providers

   Added ability to configure custom mime types in an external JSON file, such that it can be reused
   when many storage providers are deployed at the same time.

   https://github.com/cs3org/reva/pull/2613

 * Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

 * Enhancement #2696: Preferences driver refactor and cbox sql implementation

   This PR uses the updated CS3APIs which accepts a namespace in addition to a single string key to
   recognize a user preference. It also refactors the GRPC service to support multiple drivers
   and adds the cbox SQL implementation.

   https://github.com/cs3org/reva/pull/2696

 * Enhancement #2052: New CS3API datatx methods

   CS3 datatx pull model methods: PullTransfer, RetryTransfer, ListTransfers Method
   CreateTransfer removed.

   https://github.com/cs3org/reva/pull/2052

 * Enhancement #2738: Site accounts site-global settings

   This PR extends the site accounts service by adding site-global settings. These are used to
   store test user credentials that are in return used by our BBE port to perform CS3API-specific
   health checks.

   https://github.com/cs3org/reva/pull/2738

 * Enhancement #2672: Further Site Accounts improvements

   Yet another PR to update the site accounts (and Mentix): New default site ID; Include service
   type in alerts; Naming unified; Remove obsolete stuff.

   https://github.com/cs3org/reva/pull/2672

 * Enhancement #2549: Site accounts improvements

   This PR improves the site accounts: - Removed/hid API key stuff - Added quick links to the main
   panel - Made alert notifications mandatory

   https://github.com/cs3org/reva/pull/2549

 * Enhancement #2488: Cephfs support keyrings with IDs

   https://github.com/cs3org/reva/pull/2488

 * Enhancement #2514: Reuse ocs role objects in other drivers

   https://github.com/cs3org/reva/pull/2514

 * Enhancement #2752: Refactor the rest user and group provider drivers

   We now maintain our own cache for all user and group data, and periodically refresh it. A redis
   server now becomes a necessary dependency, whereas it was optional previously.

   https://github.com/cs3org/reva/pull/2752

 * Enhancement #2946: Make user share indicators read from the share provider service

   https://github.com/cs3org/reva/pull/2946


