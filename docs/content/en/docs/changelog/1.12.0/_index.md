
---
title: "v1.12.0"
linkTitle: "v1.12.0"
weight: 40
description: >
  Changelog for Reva v1.12.0 (2021-08-24)
---

Changelog for reva 1.12.0 (2021-08-24)
=======================================

The following sections list the changes in reva 1.12.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1819: Disable notifications
 * Fix #2000: Fix dependency on tests
 * Fix #1957: Fix etag propagation on deletes
 * Fix #1960: Return the updated share after updating
 * Fix #1993: Fix owncloudsql GetMD
 * Fix #1954: Fix response format of the sharees API
 * Fix #1965: Fix the file target of user and group shares
 * Fix #1956: Fix trashbin listing with depth 0
 * Fix #1987: Fix windows build
 * Fix #1990: Increase oc10 compatibility of owncloudsql
 * Fix #1978: Owner type is optional
 * Fix #1980: Propagate the etag after restoring a file version
 * Fix #1985: Add quota stubs
 * Fix #1992: Check if symlink exists instead of spamming the console
 * Fix #1913: Logic to restore files to readonly nodes
 * Chg #1982: Move user context methods into a separate `userctx` package
 * Enh #1946: Add share manager that connects to oc10 databases
 * Enh #1983: Add Codacy unit test coverage
 * Enh #1803: Introduce new webdav spaces endpoint
 * Enh #1998: Initial version of the Nextcloud storage driver
 * Enh #1984: Replace OpenCensus with OpenTelemetry
 * Enh #1861: Add support for runtime plugins
 * Enh #2008: Site account extensions

Details
-------

 * Bugfix #1819: Disable notifications

   The presence of the key `notifications` in the capabilities' response would cause clients to
   attempt to poll the notifications endpoint, which is not yet supported. To prevent the
   unnecessary bandwidth we are disabling this altogether.

   https://github.com/cs3org/reva/pull/1819

 * Bugfix #2000: Fix dependency on tests

   The Nextcloud storage driver depended on a mock http client from the tests/ folder This broke
   the Docker build The dependency was removed A check was added to test the Docker build on each PR

   https://github.com/cs3org/reva/pull/2000

 * Bugfix #1957: Fix etag propagation on deletes

   When deleting a file the etag propagation would skip the parent of the deleted file.

   https://github.com/cs3org/reva/pull/1957

 * Bugfix #1960: Return the updated share after updating

   When updating the state of a share in the in-memory share manager the old share state was
   returned instead of the updated state.

   https://github.com/cs3org/reva/pull/1960

 * Bugfix #1993: Fix owncloudsql GetMD

   The GetMD call internally was not prefixing the path when looking up resources by id.

   https://github.com/cs3org/reva/pull/1993

 * Bugfix #1954: Fix response format of the sharees API

   The sharees API wasn't returning the users and groups arrays correctly.

   https://github.com/cs3org/reva/pull/1954

 * Bugfix #1965: Fix the file target of user and group shares

   In some cases the file target of user and group shares was not properly prefixed.

   https://github.com/cs3org/reva/pull/1965
   https://github.com/cs3org/reva/pull/1967

 * Bugfix #1956: Fix trashbin listing with depth 0

   The trashbin API handled requests with depth 0 the same as request with a depth of 1.

   https://github.com/cs3org/reva/pull/1956

 * Bugfix #1987: Fix windows build

   Add the necessary `golang.org/x/sys/windows` package import to `owncloud` and
   `owncloudsql` storage drivers.

   https://github.com/cs3org/reva/pull/1987

 * Bugfix #1990: Increase oc10 compatibility of owncloudsql

   We added a few changes to the owncloudsql storage driver to behave more like oc10.

   https://github.com/cs3org/reva/pull/1990

 * Bugfix #1978: Owner type is optional

   When reading the user from the extended attributes the user type might not be set, in this case we
   now return a user with an invalid type, which correctly reflects the state on disk.

   https://github.com/cs3org/reva/pull/1978

 * Bugfix #1980: Propagate the etag after restoring a file version

   The decomposedfs didn't propagate after restoring a file version.

   https://github.com/cs3org/reva/pull/1980

 * Bugfix #1985: Add quota stubs

   The `owncloud` and `owncloudsql` drivers now read the available quota from disk to no longer
   always return 0, which causes the web UI to disable uploads.

   https://github.com/cs3org/reva/pull/1985

 * Bugfix #1992: Check if symlink exists instead of spamming the console

   The logs have been spammed with messages like `could not create symlink for ...` when using the
   decomposedfs, eg. with the oCIS storage. We now check if the link exists before trying to create
   it.

   https://github.com/cs3org/reva/pull/1992

 * Bugfix #1913: Logic to restore files to readonly nodes

   This impacts solely the DecomposedFS. Prior to these changes there was no validation when a
   user tried to restore a file from the trashbin to a share location (i.e any folder under
   `/Shares`).

   With this patch if the user restoring the resource has write permissions on the share, restore
   is possible.

   https://github.com/cs3org/reva/pull/1913

 * Change #1982: Move user context methods into a separate `userctx` package

   https://github.com/cs3org/reva/pull/1982

 * Enhancement #1946: Add share manager that connects to oc10 databases

   https://github.com/cs3org/reva/pull/1946

 * Enhancement #1983: Add Codacy unit test coverage

   This PR adds unit test coverage upload to Codacy.

   https://github.com/cs3org/reva/pull/1983

 * Enhancement #1803: Introduce new webdav spaces endpoint

   Clients can now use a new webdav endpoint
   `/dav/spaces/<storagespaceid>/relative/path/to/file` to directly access storage
   spaces.

   The `<storagespaceid>` can be retrieved using the ListStorageSpaces CS3 api call.

   https://github.com/cs3org/reva/pull/1803

 * Enhancement #1998: Initial version of the Nextcloud storage driver

   This is not usable yet in isolation, but it's a first component of
   https://github.com/pondersource/sciencemesh-nextcloud

   https://github.com/cs3org/reva/pull/1998

 * Enhancement #1984: Replace OpenCensus with OpenTelemetry

   OpenTelemetry](https://opentelemetry.io/docs/concepts/what-is-opentelemetry/) is
   an [open standard](https://github.com/open-telemetry/opentelemetry-specification) a
   sandbox CNCF project and it was formed through a merger of the OpenTracing and OpenCensus.

   > OpenCensus and OpenTracing have merged to form OpenTelemetry, which serves as the next major
   version of OpenCensus and OpenTracing. OpenTelemetry will offer backwards compatibility
   with existing OpenCensus integrations, and we will continue to make security patches to
   existing OpenCensus libraries for two years.

   There is a lot of outdated documentation as a result of this merger, and we will be better off
   adopting the latest standard and libraries.

   https://github.com/cs3org/reva/pull/1984

 * Enhancement #1861: Add support for runtime plugins

   This PR introduces a new plugin package, that allows loading external plugins into Reva at
   runtime. The hashicorp go-plugin framework was used to facilitate the plugin loading and
   communication.

   https://github.com/cs3org/reva/pull/1861

 * Enhancement #2008: Site account extensions

   This PR heavily extends the site accounts service: * Extended the accounts information (not
   just email and name) * Accounts now have a password * Users can now "log in" to their accounts and
   edit it * Ability to grant access to the GOCDB

   Furthermore, these accounts can now be used to authenticate for logging in to our customized
   GOCDB. More use cases for these accounts are also planned.

   https://github.com/cs3org/reva/pull/2008


