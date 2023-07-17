
---
title: "v2.15.0"
linkTitle: "v2.15.0"
weight: 40
description: >
  Changelog for Reva v2.15.0 (2023-07-17)
---

Changelog for reva 2.15.0 (2023-07-17)
=======================================

The following sections list the changes in reva 2.15.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4004: Add path to public link POST
*   Fix #3993: Add token to LinkAccessedEvent
*   Fix #4007: Close archive writer properly
*   Fix #3982: Fixed couple of smaller space lookup issues
*   Fix #3963: Treesize interger overflows
*   Fix #3943: When removing metadata always use correct database and table
*   Fix #4003: Don't connect ldap on startup
*   Fix #3978: Decomposedfs no longer os.Stats when reading node metadata
*   Fix #3959: Drop unnecessary stat
*   Fix #4032: Temporarily exclude ceph-iscsi when building revad-ceph image
*   Fix #4042: Fix writing 0 byte msgpack metadata
*   Fix #3948: Handle the bad request status
*   Fix #3970: Fix enforce-password issue
*   Fix #4057: Properly handle not-found errors when getting a public share
*   Fix #4048: Fix messagepack propagation
*   Fix #4056: Fix destroys data destination when moving issue
*   Fix #4012: Fix mtime if 0 size file uploaded
*   Fix #3955: Fix panic
*   Fix #3977: Prevent direct access to trash items
*   Fix #3933: Concurrently invalidate mtime cache in jsoncs3 share manager
*   Fix #3985: Reduce jsoncs3 lock congestion
*   Fix #3960: Add trace span details
*   Fix #3951: Link context in metadata client
*   Fix #4010: Omit spaceroot when archiving
*   Fix #3950: Use plain otel tracing in metadata client
*   Fix #3975: Decomposedfs now resolves the parent without an os.Stat
*   Fix #4047: Publish events synchrously
*   Fix #4039: Restart Postprocessing
*   Chg #3947: Bump golangci-lint to 1.51.2
*   Chg #3945: Revert golangci-lint back to 1.50.1
*   Enh #4060: We added a go-micro based app-provider registry
*   Enh #4013: Add new WebDAV permissions
*   Enh #3966: Add space metadata to ocs shares list
*   Enh #3987: Cache space indexes
*   Enh #3953: Client selector pool
*   Enh #3973: More logging for metadata propagation
*   Enh #4059: Improve space index performance
*   Enh #3994: Load matching spaces concurrently
*   Enh #4049: Do not invalidate filemetadata cache early
*   Enh #4040: Allow to use external trace provider in micro service
*   Enh #4019: Allow to use external trace provider
*   Enh #4045: Log error message in grpc interceptor
*   Enh #3989: Parallelization of jsoncs3 operations
*   Enh #3941: Adding tracing for jsoncs3
*   Enh #3965: ResumePostprocessing Event
*   Enh #3809: Trace decomposedfs syscalls
*   Enh #4067: Trace upload progress
*   Enh #3887: Trace requests through datagateway
*   Enh #3981: We have updated the UserFeatureChangedEvent to reflect value changes
*   Enh #4052: Update go-ldap to v3.4.5
*   Enh #4065: Upload directly to dataprovider
*   Enh #4046: Use correct tracer name
*   Enh #3986: Allow disabling wopi chat

Details
-------

*   Bugfix #4004: Add path to public link POST

   When POSTing a public link, the response would not contain the complete path to the resource.
   This is fixed now.

   https://github.com/cs3org/reva/pull/4004

*   Bugfix #3993: Add token to LinkAccessedEvent

   We added the link token to the LinkAccessedEvent

   https://github.com/owncloud/ocis/issues/3753
   https://github.com/cs3org/reva/pull/3993

*   Bugfix #4007: Close archive writer properly

   When running into max size error (or random other error) the archiver would not close the
   writer. Only it success case it would. This resulted in broken archives on the client. We now
   `defer` the writer close.

   https://github.com/cs3org/reva/pull/4007

*   Bugfix #3982: Fixed couple of smaller space lookup issues

   https://github.com/cs3org/reva/pull/3982

*   Bugfix #3963: Treesize interger overflows

   Reading the treesize was parsing the string value as a signed integer while setting the
   treesize used unsigned integers this could cause failures (out of range errors) when reading
   very large treesizes.

   https://github.com/cs3org/reva/pull/3963

*   Bugfix #3943: When removing metadata always use correct database and table

   https://github.com/cs3org/reva/pull/3943

*   Bugfix #4003: Don't connect ldap on startup

   This leads to misleading error messages. Instead connect on first request.

   https://github.com/cs3org/reva/pull/4003

*   Bugfix #3978: Decomposedfs no longer os.Stats when reading node metadata

   https://github.com/cs3org/reva/pull/3978

*   Bugfix #3959: Drop unnecessary stat

   https://github.com/cs3org/reva/pull/3959

*   Bugfix #4032: Temporarily exclude ceph-iscsi when building revad-ceph image

   Due to `Package ceph-iscsi-3.6-1.el8.noarch.rpm is not signed` error when building the
   revad-ceph docker image, the package `ceph-iscsi` has been excluded from the dnf update. It
   will be included again once the pkg will be signed again.

   https://github.com/cs3org/reva/pull/4032

*   Bugfix #4042: Fix writing 0 byte msgpack metadata

   File metadata is now written atomically to be more resilient during timeouts

   https://github.com/cs3org/reva/pull/4042
   https://github.com/cs3org/reva/pull/4034
   https://github.com/cs3org/reva/pull/4033

*   Bugfix #3948: Handle the bad request status

   Handle the bad request status for the CreateStorageSpace function

   https://github.com/cs3org/reva/pull/3948

*   Bugfix #3970: Fix enforce-password issue

   Fix updating public share without password when enforce-password is enabled

   https://github.com/owncloud/ocis/issues/6476
   https://github.com/cs3org/reva/pull/3970

*   Bugfix #4057: Properly handle not-found errors when getting a public share

   We fixed a problem where not-found errors caused a hard error instead of a proper RPC error
   state.

   https://github.com/cs3org/reva/pull/4057

*   Bugfix #4048: Fix messagepack propagation

   We cannot read from the lockfile. The data is in the metadata file.

   https://github.com/cs3org/reva/pull/4048

*   Bugfix #4056: Fix destroys data destination when moving issue

   Fix moving a file and providing the fileID of the destination destroys data

   https://github.com/owncloud/ocis/issues/6739
   https://github.com/cs3org/reva/pull/4056

*   Bugfix #4012: Fix mtime if 0 size file uploaded

   Fix mtime if 0 size file uploaded

   https://github.com/owncloud/ocis/issues/1248
   https://github.com/cs3org/reva/pull/4012

*   Bugfix #3955: Fix panic

   https://github.com/cs3org/reva/pull/3955

*   Bugfix #3977: Prevent direct access to trash items

   Decomposedfs now prevents direct access to trash items

   https://github.com/cs3org/reva/pull/3977

*   Bugfix #3933: Concurrently invalidate mtime cache in jsoncs3 share manager

   https://github.com/cs3org/reva/pull/3933

*   Bugfix #3985: Reduce jsoncs3 lock congestion

   We changed the locking strategy in the jsoncs3 share manager to cause less lock congestion
   increasing the performance in certain scenarios.

   https://github.com/cs3org/reva/pull/3985
   https://github.com/cs3org/reva/pull/3964

*   Bugfix #3960: Add trace span details

   https://github.com/cs3org/reva/pull/3960

*   Bugfix #3951: Link context in metadata client

   We now disconnect the existing outgoing grpc metadata when making calls in the metadata
   client. To keep track of related spans we link the two contexts.

   https://github.com/cs3org/reva/pull/3951

*   Bugfix #4010: Omit spaceroot when archiving

   When archiving a space there was an empty folder named `.` added. This was because of the
   spaceroot which was wrongly interpreted. We now omit the space root when creating an archive.

   https://github.com/cs3org/reva/pull/4010
   https://github.com/cs3org/reva/pull/3999

*   Bugfix #3950: Use plain otel tracing in metadata client

   In some cases there was no tracer provider in the context. Since the otel tracing has settled we
   will fix problems by moving to the recommended best practices. A good starting point is
   https://lightstep.com/blog/opentelemetry-go-all-you-need-to-know

   https://github.com/cs3org/reva/pull/3950

*   Bugfix #3975: Decomposedfs now resolves the parent without an os.Stat

   https://github.com/cs3org/reva/pull/3975

*   Bugfix #4047: Publish events synchrously

   Async publishing can lead to loss of events under some circumstances

   https://github.com/cs3org/reva/pull/4047

*   Bugfix #4039: Restart Postprocessing

   Resend the `BytesReady` event if instructed.

   https://github.com/cs3org/reva/pull/4039

*   Change #3947: Bump golangci-lint to 1.51.2

   The 1.50.1 release had memory issues with go1.20

   https://github.com/cs3org/reva/pull/3947

*   Change #3945: Revert golangci-lint back to 1.50.1

   https://github.com/cs3org/reva/pull/3945

*   Enhancement #4060: We added a go-micro based app-provider registry

   We added a dynamic go-micro based app-provider registry with a dynamic TTL

   https://github.com/owncloud/ocis/issues/3832
   https://github.com/cs3org/reva/pull/4060

*   Enhancement #4013: Add new WebDAV permissions

   We added the permission "PurgeRecycle" to the WebDAV permissions list. I is represented by the
   capital letter `P`.

   https://github.com/cs3org/reva/pull/4013

*   Enhancement #3966: Add space metadata to ocs shares list

   We needed to add the space ID and the space alias of the original space to the share metadata when
   we are listing shares. This is needed to directly navigate to the originating space location.

   https://github.com/cs3org/reva/pull/3966

*   Enhancement #3987: Cache space indexes

   Decomposedfs now caches the different space indexes in memory which greatly improves the
   performance of ListStorageSpaces on slow storages.

   https://github.com/cs3org/reva/pull/3987

*   Enhancement #3953: Client selector pool

   Add the ability to use iterable client pools for the grpc service communication, the
   underlying grpc client and connection is fetched randomly from the available services.

   https://github.com/cs3org/reva/pull/3953
   https://github.com/cs3org/reva/pull/3939

*   Enhancement #3973: More logging for metadata propagation

   In order to detect possible issues with the treesize propagation we made the logging a bit more
   verbose.

   https://github.com/cs3org/reva/pull/3973

*   Enhancement #4059: Improve space index performance

   The directory tree based decomposedfs space indexes have been replaced with messagepack base
   indexes which improves performance when reading the index, especially on slow storages.

   https://github.com/cs3org/reva/pull/4059
   https://github.com/cs3org/reva/pull/4058
   https://github.com/cs3org/reva/pull/3995

*   Enhancement #3994: Load matching spaces concurrently

   Matching spaces in a ListStorageSpace call are now loaded concurrently which reduces the
   response time.

   https://github.com/cs3org/reva/pull/3994

*   Enhancement #4049: Do not invalidate filemetadata cache early

   We can postpone overwriting the cache until the metadata has ben written to disk. This prevents
   other requests trying to read metadata from having to wait for a readlock for the metadata file.

   https://github.com/cs3org/reva/pull/4049

*   Enhancement #4040: Allow to use external trace provider in micro service

   Allow injecting of external trace provider in the micro service instead of forcing the
   initialisation of an internal one.

   https://github.com/cs3org/reva/pull/4040

*   Enhancement #4019: Allow to use external trace provider

   Allow injecting of external trace provider instead of forcing the initialisation of an
   internal one.

   https://github.com/cs3org/reva/pull/4019

*   Enhancement #4045: Log error message in grpc interceptor

   The grpc log interceptor now logs the actual error message

   https://github.com/cs3org/reva/pull/4045

*   Enhancement #3989: Parallelization of jsoncs3 operations

   Run removeShare and share create storage operations in parallel.

   https://github.com/cs3org/reva/pull/3989

*   Enhancement #3941: Adding tracing for jsoncs3

   https://github.com/cs3org/reva/pull/3941

*   Enhancement #3965: ResumePostprocessing Event

   Add a new event: `ResumePostprocessing`. It can be emitted to repair broken postprocessing

   https://github.com/cs3org/reva/pull/3965

*   Enhancement #3809: Trace decomposedfs syscalls

   To investigate performance characteristics of the underlying storage when the system is
   under load we wrapped the related syscalls in decomposedfs with trace spans.

   https://github.com/cs3org/reva/pull/3809

*   Enhancement #4067: Trace upload progress

   https://github.com/cs3org/reva/pull/4067

*   Enhancement #3887: Trace requests through datagateway

   The datagateway now forwards tracing headers.

   https://github.com/cs3org/reva/pull/3887

*   Enhancement #3981: We have updated the UserFeatureChangedEvent to reflect value changes

   A UserFeatureChanged Event can now contain an old and a new value to reflect value changes in
   audit logs.

   https://github.com/owncloud/ocis/issues/3753
   https://github.com/cs3org/reva/pull/3981

*   Enhancement #4052: Update go-ldap to v3.4.5

   Updated go-ldap/ldap/v3 to the latest upstream release to get back to a released version (we
   were targeting a specific bugfix commit previously)

   https://github.com/cs3org/reva/pull/4052

*   Enhancement #4065: Upload directly to dataprovider

   The ocdav service can now bypass the datagateway if it is configured with a transfer secret.
   This prevents unnecessary roundtrips and halves the network traffic during uploads for the
   proxy.

   https://github.com/owncloud/ocis/issues/6296
   https://github.com/cs3org/reva/pull/4065

*   Enhancement #4046: Use correct tracer name

   https://github.com/cs3org/reva/pull/4046

*   Enhancement #3986: Allow disabling wopi chat

   Allow disabling the chat in wopi (support for onlyoffice only)

   https://github.com/cs3org/reva/pull/3986

