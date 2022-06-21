
---
title: "v2.6.0"
linkTitle: "v2.6.0"
weight: 40
description: >
  Changelog for Reva v2.6.0 (2022-06-21)
---

Changelog for reva 2.6.0 (2022-06-21)
=======================================

The following sections list the changes in reva 2.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2985: Make stat requests route based on storage providerid
 * Fix #2987: Let archiver handle all error codes
 * Fix #2994: Fix errors when loading shares
 * Fix #2996: Do not close share dump channels
 * Fix #2993: Remove unused configuration
 * Fix #2950: Fix sharing with space ref
 * Fix #2991: Make sharesstorageprovider get accepted share
 * Chg #2877: Enable resharing
 * Chg #2984: Update CS3Apis
 * Enh #3753: Add executant to the events
 * Enh #2820: Instrument GRPC and HTTP requests with OTel
 * Enh #2975: Leverage shares space storageid and type when listing shares
 * Enh #3882: Explicitly return on ocdav move requests with body
 * Enh #2932: Stat accepted shares mountpoints, configure existing share updates
 * Enh #2944: Improve owncloudsql connection management
 * Enh #2962: Per service TracerProvider
 * Enh #2911: Allow for dumping and loading shares
 * Enh #2938: Sharpen tooling

Details
-------

 * Bugfix #2985: Make stat requests route based on storage providerid

   The gateway now uses a filter mask to only fetch the root id of a space for stat requests. This
   allows the spaces registry to determine the responsible storage provider without querying
   the storageproviders.

   https://github.com/cs3org/reva/pull/2985

 * Bugfix #2987: Let archiver handle all error codes

   We fixed the archiver handler to handle all error codes

   https://github.com/cs3org/reva/pull/2987

 * Bugfix #2994: Fix errors when loading shares

   We fixed a bug where loading shares and associated received shares ran into issues when
   handling them simultaneously.

   https://github.com/cs3org/reva/pull/2994

 * Bugfix #2996: Do not close share dump channels

   We no longer close the channels when dumping shares, it's the responsibility of the caller.

   https://github.com/cs3org/reva/pull/2996

 * Bugfix #2993: Remove unused configuration

   We've fixed removed unused configuration:

   - `insecure` from the dataprovider - `timeout` from the dataprovider - `tmp_folder` from the
   storageprovider

   https://github.com/cs3org/reva/pull/2993

 * Bugfix #2950: Fix sharing with space ref

   We've fixed a bug where share requests with `path` attribute present ignored the `space_ref`
   attribute. We now give the `space_ref` attribute precedence over the `path` attribute.

   https://github.com/cs3org/reva/pull/2950

 * Bugfix #2991: Make sharesstorageprovider get accepted share

   The sharesstorageprovider now gets an accepted share instead of filtering all shares.

   https://github.com/cs3org/reva/pull/2991

 * Change #2877: Enable resharing

   This will allow resharing of files. - All Viewers and Editors are now able to reshare files and
   folders - One can still edit their own shares, even when loosing share permissions - Viewers and
   Editors in a space are not affected

   https://github.com/cs3org/reva/pull/2877

 * Change #2984: Update CS3Apis

   Updated the CS3Apis to make use of field_mask and pagination for list requests.

   https://github.com/cs3org/reva/pull/2984

 * Enhancement #3753: Add executant to the events

   Added the executant field to all events.

   https://github.com/owncloud/ocis/issues/3753
   https://github.com/cs3org/reva/pull/2945

 * Enhancement #2820: Instrument GRPC and HTTP requests with OTel

   We've added the enduser.id tag to the HTTP and GRPC requests. We've fixed the tracer names.
   We've decorated the traces with the hostname.

   https://github.com/cs3org/reva/pull/2820

 * Enhancement #2975: Leverage shares space storageid and type when listing shares

   The list shares call now also fills the storageid to allow the space registry to directly route
   requests to the correct storageprovider. The spaces registry will now also skip
   storageproviders that are not configured for a requested type, causing type 'personal'
   requests to skip the sharestorageprovider.

   https://github.com/cs3org/reva/pull/2975
   https://github.com/cs3org/reva/pull/2980

 * Enhancement #3882: Explicitly return on ocdav move requests with body

   Added a check if a ocdav move request contains a body. If it does a 415 415 (Unsupported Media
   Type) will be returned.

   https://github.com/owncloud/ocis/issues/3882
   https://github.com/cs3org/reva/pull/2974

 * Enhancement #2932: Stat accepted shares mountpoints, configure existing share updates

   https://github.com/cs3org/reva/pull/2932

 * Enhancement #2944: Improve owncloudsql connection management

   The owncloudsql storagedriver is now aware of the request context and will close db
   connections when http connections are closed or time out. We also increased the max number of
   open connections from 10 to 100 to prevent a corner case where all connections were used but idle
   connections were not freed.

   https://github.com/cs3org/reva/pull/2944

 * Enhancement #2962: Per service TracerProvider

   To improve tracing we create separate TracerProviders per service now. This is especially
   helpful when running multiple reva services in a single process (like e.g. oCIS does).

   https://github.com/cs3org/reva/pull/2962
   https://github.com/cs3org/reva/pull/2978

 * Enhancement #2911: Allow for dumping and loading shares

   We now have interfaces for dumpable and loadable share manages which can be used to migrate
   shares between share managers

   https://github.com/cs3org/reva/pull/2911

 * Enhancement #2938: Sharpen tooling

   * We increased the linting timeout to 10min which caused some release builds to time out

   https://github.com/cs3org/reva/pull/2938


