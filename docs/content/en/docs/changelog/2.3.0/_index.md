
---
title: "v2.3.0"
linkTitle: "v2.3.0"
weight: 40
description: >
  Changelog for Reva v2.3.0 (2022-05-02)
---

Changelog for reva 2.3.0 (2022-05-02)
=======================================

The following sections list the changes in reva 2.3.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2693: Support editnew actions from MS Office
 * Fix #2588: Dockerfile.revad-ceph to use the right base image
 * Fix #2499: Removed check DenyGrant in resource permission
 * Fix #2285: Accept new userid idp format
 * Fix #2802: Fix the resource id handling for space shares
 * Fix #2800: Fix spaceid parsing in spaces trashbin API
 * Fix #2608: Respect the tracing_service_name config variable
 * Fix #2742: Use exact match in login filter
 * Fix #2759: Made uid, gid claims parsing more robust in OIDC auth provider
 * Fix #2788: Return the correct file IDs on public link resources
 * Fix #2322: Use RFC3339 for parsing dates
 * Fix #2784: Disable storageprovider cache for the share jail
 * Fix #2555: Fix site accounts endpoints
 * Fix #2675: Updates Makefile according to latest go standards
 * Fix #2572: Wait for nats server on middleware start
 * Chg #2735: Avoid user enumeration
 * Chg #2737: Bump go-cs3api
 * Chg #2763: Change the oCIS and S3NG  storage driver blob store layout
 * Chg #2596: Remove hash from public link urls
 * Chg #2785: Implement workaround for chi.RegisterMethod
 * Chg #2559: Do not encode webDAV ids to base64
 * Chg #2740: Rename oc10 share manager driver
 * Chg #2561: Merge oidcmapping auth manager into oidc
 * Enh #2698: Make capabilities endpoint public, authenticate users is present
 * Enh #2515: Enabling tracing by default if not explicitly disabled
 * Enh #2686: Features for favorites xattrs in EOS, cache for scope expansion
 * Enh #2494: Use sys ACLs for file permissions
 * Enh #2522: Introduce events
 * Enh #2811: Add event for created directories
 * Enh #2798: Add additional fields to events to enable search
 * Enh #2790: Fake providerids so API stays stable after beta
 * Enh #2685: Enable federated account access
 * Enh #1787: Add support for HTTP TPC
 * Enh #2799: Add flag to enable unrestriced listing of spaces
 * Enh #2560: Mentix PromSD extensions
 * Enh #2741: Meta path for user
 * Enh #2613: Externalize custom mime types configuration for storage providers
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2696: Preferences driver refactor and cbox sql implementation
 * Enh #2052: New CS3API datatx methods
 * Enh #2743: Add capability for public link single file edit
 * Enh #2738: Site accounts site-global settings
 * Enh #2672: Further Site Accounts improvements
 * Enh #2549: Site accounts improvements
 * Enh #2795: Add feature flags "projects" and "share_jail" to spaces capability
 * Enh #2514: Reuse ocs role objects in other drivers
 * Enh #2781: In memory user provider
 * Enh #2752: Refactor the rest user and group provider drivers

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

 * Bugfix #2499: Removed check DenyGrant in resource permission

   When adding a denial permission

   https://github.com/cs3org/reva/pull/2499

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

 * Bugfix #2802: Fix the resource id handling for space shares

   Adapt the space shares to the new id format.

   https://github.com/cs3org/reva/pull/2802

 * Bugfix #2800: Fix spaceid parsing in spaces trashbin API

   Added proper space id parsing to the spaces trashbin API endpoint.

   https://github.com/cs3org/reva/pull/2800

 * Bugfix #2608: Respect the tracing_service_name config variable

   https://github.com/cs3org/reva/pull/2608

 * Bugfix #2742: Use exact match in login filter

   After the recent config changes the auth-provider was accidently using a substring match for
   the login filter. It's no fixed to use an exact match.

   https://github.com/cs3org/reva/pull/2742

 * Bugfix #2759: Made uid, gid claims parsing more robust in OIDC auth provider

   This fix makes sure the uid and gid claims are defined at init time, and that the necessary
   typecasts are performed correctly when authenticating users. A comment was added that in case
   the uid/gid claims are missing AND that no mapping takes place, a user entity is returned with
   uid = gid = 0.

   https://github.com/cs3org/reva/pull/2759

 * Bugfix #2788: Return the correct file IDs on public link resources

   Resources in public shares should return the real resourceids from the storage of the owner.

   https://github.com/cs3org/reva/pull/2788

 * Bugfix #2322: Use RFC3339 for parsing dates

   We have used the RFC3339 format for parsing dates to be consistent with oC Web.

   https://github.com/cs3org/reva/issues/2322
   https://github.com/cs3org/reva/pull/2744

 * Bugfix #2784: Disable storageprovider cache for the share jail

   The share jail should not be cached in the provider cache because it is a virtual collection of
   resources from different storage providers.

   https://github.com/cs3org/reva/pull/2784

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

 * Change #2735: Avoid user enumeration

   Sending PROPFIND requests to `../files/admin` did return a different response than sending
   the same request to `../files/notexists`. This allowed enumerating users. This response was
   changed to be the same always

   https://github.com/cs3org/reva/pull/2735

 * Change #2737: Bump go-cs3api

   Bumped version of the go-cs3api

   https://github.com/cs3org/reva/pull/2737

 * Change #2763: Change the oCIS and S3NG  storage driver blob store layout

   We've optimized the oCIS and S3NG storage driver blob store layout.

   For the oCIS storage driver, blobs will now be stored inside the folder of a space, next to the
   nodes. This allows admins to easily archive, backup and restore spaces as a whole with UNIX
   tooling. We also moved from a single folder for blobs to multiple folders for blobs, to make the
   filesystem interactions more performant for large numbers of files.

   The previous layout on disk looked like this:

   ```markdown |-- spaces | |-- .. | | |-- .. | |-- xx | |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <-
   partitioned space id | |-- nodes | |-- .. | |-- xx | |-- xx | |-- xx | |-- xx | |--
   -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id |-- blobs |-- .. |--
   xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id ```

   Now it looks like this:

   ```markdown |-- spaces | |-- .. | | |-- .. |-- xx |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <-
   partitioned space id |-- nodes | |-- .. | |-- xx | |-- xx | |-- xx | |-- xx | |--
   -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id |-- blobs |-- .. |-- xx |-- xx |-- xx |-- xx
   |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id ```

   For the S3NG storage driver, blobs will now be prefixed with the space id and also a part of the
   blob id will be used as prefix. This creates a better prefix partitioning and mitigates S3 api
   performance drops for large buckets
   (https://aws.amazon.com/de/premiumsupport/knowledge-center/s3-prefix-nested-folders-difference/).

   The previous S3 bucket (blobs only looked like this):

   ```markdown |-- .. |-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id ```

   Now it looks like this:

   ```markdown |-- .. |-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- space id |-- .. |-- xx |-- xx
   |-- xx |-- xx |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id ```

   https://github.com/owncloud/ocis/issues/3557
   https://github.com/cs3org/reva/pull/2763

 * Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

 * Change #2785: Implement workaround for chi.RegisterMethod

   Implemented a workaround for `chi.RegisterMethod` because of a concurrent map read write
   issue. This needs to be fixed upstream in go-chi.

   https://github.com/cs3org/reva/pull/2785

 * Change #2559: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2559

 * Change #2740: Rename oc10 share manager driver

   We aligned the oc10 SQL share manager driver name with all other owncloud spacific SQL drivers
   by renaming the package `pkg/share/manager/sql` to `pkg/share/manager/owncloudsql` and
   changing the name from `oc10-sql` to `owncloudsql`.

   https://github.com/cs3org/reva/pull/2740

 * Change #2561: Merge oidcmapping auth manager into oidc

   The oidcmapping auth manager was created as a separate package to ease testing. As it has now
   been tested also as a pure OIDC auth provider without mapping, and as the code is largely
   refactored, it makes sense to merge it back so to maintain a single OIDC manager.

   https://github.com/cs3org/reva/pull/2561

 * Enhancement #2698: Make capabilities endpoint public, authenticate users is present

   https://github.com/cs3org/reva/pull/2698

 * Enhancement #2515: Enabling tracing by default if not explicitly disabled

   https://github.com/cs3org/reva/pull/2515

 * Enhancement #2686: Features for favorites xattrs in EOS, cache for scope expansion

   https://github.com/cs3org/reva/pull/2686

 * Enhancement #2494: Use sys ACLs for file permissions

   https://github.com/cs3org/reva/pull/2494

 * Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

 * Enhancement #2811: Add event for created directories

   We added another event for created directories.

   https://github.com/cs3org/reva/pull/2811

 * Enhancement #2798: Add additional fields to events to enable search

   https://github.com/cs3org/reva/pull/2798

 * Enhancement #2790: Fake providerids so API stays stable after beta

   To support the stativ registry, we need to accept providerids This fakes the ids so the API can
   stay stable

   https://github.com/cs3org/reva/pull/2790

 * Enhancement #2685: Enable federated account access

   https://github.com/cs3org/reva/pull/2685

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

 * Enhancement #2799: Add flag to enable unrestriced listing of spaces

   Listing spaces now only returns all spaces when the user has the permissions and it was
   explicitly requested. The default will only return the spaces the current user has access to.

   https://github.com/cs3org/reva/pull/2799

 * Enhancement #2560: Mentix PromSD extensions

   The Mentix Prometheus SD scrape targets are now split into one file per service type, making
   health checks configuration easier. Furthermore, the local file connector for mesh data and
   the site registration endpoint have been dropped, as they aren't needed anymore.

   https://github.com/cs3org/reva/pull/2560

 * Enhancement #2741: Meta path for user

   We've added support for requesting the `meta-path-for-user` via a propfind to the
   `dav/meta/<id>` endpoint.

   https://github.com/cs3org/reva/pull/2741
   https://github.com/cs3org/reva/pull/2793
   https://doc.owncloud.com/server/next/developer_manual/webdav_api/meta.html

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

 * Enhancement #2743: Add capability for public link single file edit

   It is now possible to share a single file by link with edit permissions. Therefore we need a
   public share capability to enable that feature in the clients. At the same time we improved the
   WebDAV permissions for public links.

   https://github.com/cs3org/reva/pull/2743

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

 * Enhancement #2795: Add feature flags "projects" and "share_jail" to spaces capability

   https://github.com/cs3org/reva/pull/2795

 * Enhancement #2514: Reuse ocs role objects in other drivers

   https://github.com/cs3org/reva/pull/2514

 * Enhancement #2781: In memory user provider

   We added an in memory implementation for the user provider that reads the users from the
   mapstructure passed in.

   https://github.com/cs3org/reva/pull/2781

 * Enhancement #2752: Refactor the rest user and group provider drivers

   We now maintain our own cache for all user and group data, and periodically refresh it. A redis
   server now becomes a necessary dependency, whereas it was optional previously.

   https://github.com/cs3org/reva/pull/2752


