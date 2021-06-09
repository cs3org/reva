
---
title: "v1.8.0"
linkTitle: "v1.8.0"
weight: 40
description: >
  Changelog for Reva v1.8.0 (2021-06-09)
---

Changelog for reva 1.8.0 (2021-06-09)
=======================================

The following sections list the changes in reva 1.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1779: Set Content-Type header correctly for ocs requests
 * Fix #1650: Allow fetching shares as the grantee
 * Fix #1693: Fix move in owncloud storage driver
 * Fix #1666: Fix public file shares
 * Fix #1541: Allow for restoring recycle items to different locations
 * Fix #1718: Use the -static ldflag only for the 'build-ci' target
 * Enh #1719: Application passwords CLI
 * Enh #1719: Application passwords management
 * Enh #1725: Create transfer type share
 * Enh #1755: Return file checksum available from the metadata for the EOS driver
 * Enh #1673: Deprecate using errors.New and fmt.Errorf
 * Enh #1723: Open in app workflow using the new API
 * Enh #1655: Improve json marshalling of share protobuf messages
 * Enh #1694: User profile picture capability
 * Enh #1649: Add reliability calculations support to Mentix
 * Enh #1509: Named Service Registration
 * Enh #1643: Cache resources from share getter methods in OCS
 * Enh #1664: Add cache warmup strategy for OCS resource infos
 * Enh #1710: Owncloudsql storage driver
 * Enh #1705: Reduce the size of all the container images built on CI
 * Enh #1669: Mint scope-based access tokens for RBAC
 * Enh #1683: Filter created shares based on type in OCS
 * Enh #1763: Sort share entries alphabetically
 * Enh #1758: Warn user for not recommended go version
 * Enh #1747: Add checksum headers to tus preflight responses
 * Enh #1685: Add share to update response

Details
-------

 * Bugfix #1779: Set Content-Type header correctly for ocs requests

   Before this fix the `Content-Type` header was guessed by `w.Write` because `WriteHeader` was
   called to early. Now the `Content-Type` is set correctly and to the same values as in ownCloud 10

   https://github.com/owncloud/ocis/issues/1779

 * Bugfix #1650: Allow fetching shares as the grantee

   The json backend now allows a grantee to fetch a share by id.

   https://github.com/cs3org/reva/pull/1650

 * Bugfix #1693: Fix move in owncloud storage driver

   When moving a file or folder (includes renaming) the filepath in the cache didn't get updated
   which caused subsequent requests to `getpath` to fail.

   https://github.com/cs3org/reva/issues/1693
   https://github.com/cs3org/reva/issues/1696

 * Bugfix #1666: Fix public file shares

   Fixed stat requests and propfind responses for publicly shared files.

   https://github.com/cs3org/reva/pull/1666

 * Bugfix #1541: Allow for restoring recycle items to different locations

   The CS3 APIs specify a way to restore a recycle item to a different location than the original by
   setting the `restore_path` field in the `RestoreRecycleItemRequest`. This field had not
   been considered until now.

   https://github.com/cs3org/reva/pull/1541
   https://cs3org.github.io/cs3apis/

 * Bugfix #1718: Use the -static ldflag only for the 'build-ci' target

   It is not intended to statically link the generated binaries for local development workflows.
   This resulted on segmentation faults and compiller warnings.

   https://github.com/cs3org/reva/pull/1718

 * Enhancement #1719: Application passwords CLI

   This PR adds the CLI commands `token-list`, `token-create` and `token-remove` to manage
   tokens with limited scope on behalf of registered users.

   https://github.com/cs3org/reva/pull/1719

 * Enhancement #1719: Application passwords management

   This PR adds the functionality to generate authentication tokens with limited scope on behalf
   of registered users. These can be used in third party apps or in case primary user credentials
   cannot be submitted to other parties.

   https://github.com/cs3org/reva/issues/1714
   https://github.com/cs3org/reva/pull/1719
   https://github.com/cs3org/cs3apis/pull/127

 * Enhancement #1725: Create transfer type share

   `transfer-create` creates a share of type transfer.

   https://github.com/cs3org/reva/pull/1725

 * Enhancement #1755: Return file checksum available from the metadata for the EOS driver

   https://github.com/cs3org/reva/pull/1755

 * Enhancement #1673: Deprecate using errors.New and fmt.Errorf

   Previously we were using errors.New and fmt.Errorf to create errors. Now we use the errors
   defined in the errtypes package.

   https://github.com/cs3org/reva/issues/1673

 * Enhancement #1723: Open in app workflow using the new API

   This provides a new `open-in-app` command for the CLI and the implementation on the
   appprovider gateway service for the new API, including the option to specify the appplication
   to use, thus overriding the preconfigured one.

   https://github.com/cs3org/reva/pull/1723

 * Enhancement #1655: Improve json marshalling of share protobuf messages

   Protobuf oneof fields cannot be properly handled by the native json marshaller, and the
   protojson package can only handle proto messages. Previously, we were using a workaround of
   storing these oneof fields separately, which made the code inelegant. Now we marshal these
   messages as strings before marshalling them via the native json package.

   https://github.com/cs3org/reva/pull/1655

 * Enhancement #1694: User profile picture capability

   Based on feedback in the new ownCloud web frontend we want to omit trying to render user avatars
   images / profile pictures based on the backend capabilities. Now the OCS communicates a
   corresponding value.

   https://github.com/cs3org/reva/pull/1694

 * Enhancement #1649: Add reliability calculations support to Mentix

   To make reliability calculations possible, a new exporter has been added to Mentix that reads
   scheduled downtimes from the GOCDB and exposes it through Prometheus metrics.

   https://github.com/cs3org/reva/pull/1649

 * Enhancement #1509: Named Service Registration

   Move away from hardcoding service IP addresses and rely upon name resolution instead. It
   delegates the address lookup to a static in-memory service registry, which can be
   re-implemented in multiple forms.

   https://github.com/cs3org/reva/pull/1509

 * Enhancement #1643: Cache resources from share getter methods in OCS

   In OCS, once we retrieve the shares from the shareprovider service, we stat each of those
   separately to obtain the required info, which introduces a lot of latency. This PR introduces a
   resoource info cache in OCS, which would prevent this latency.

   https://github.com/cs3org/reva/pull/1643

 * Enhancement #1664: Add cache warmup strategy for OCS resource infos

   Recently, a TTL cache was added to OCS to store statted resource infos. This PR adds an interface
   to define warmup strategies and also adds a cbox specific strategy which starts a goroutine to
   initialize the cache with all the valid shares present in the system.

   https://github.com/cs3org/reva/pull/1664

 * Enhancement #1710: Owncloudsql storage driver

   This PR adds a storage driver which connects to a oc10 storage backend (storage + database).
   This allows for running oc10 and ocis with the same backend in parallel.

   https://github.com/cs3org/reva/pull/1710

 * Enhancement #1705: Reduce the size of all the container images built on CI

   Previously, all images were based on golang:1.16 which is built from Debian. Using 'scratch'
   as base, reduces the size of the artifacts well as the attack surface for all the images, plus
   copying the binary from the build step ensures that only the strictly required software is
   present on the final image. For the revad images tagged '-eos', eos-slim is used instead. It is
   still large but it updates the environment as well as the EOS version.

   https://github.com/cs3org/reva/pull/1705

 * Enhancement #1669: Mint scope-based access tokens for RBAC

   Primarily, this PR is meant to introduce the concept of scopes into our tokens. At the moment, it
   addresses those cases where we impersonate other users without allowing the full scope of what
   the actual user has access to.

   A short explanation for how it works for public shares: - We get the public share using the token
   provided by the client. - In the public share, we know the resource ID, so we can add this to the
   allowed scope, but not the path. - However, later OCDav tries to access by path as well. Now this
   is not allowed at the moment. However, from the allowed scope, we have the resource ID and we're
   allowed to stat that. We stat the resource ID, get the path and if the path matches the one passed
   by OCDav, we allow the request to go through.

   https://github.com/cs3org/reva/pull/1669

 * Enhancement #1683: Filter created shares based on type in OCS

   https://github.com/cs3org/reva/pull/1683

 * Enhancement #1763: Sort share entries alphabetically

   When showing the list of shares to the end-user, the list was not sorted alphabetically. This PR
   sorts the list of users and groups.

   https://github.com/cs3org/reva/issues/1763

 * Enhancement #1758: Warn user for not recommended go version

   This PR adds a warning while an user is building the source code, if he is using a go version not
   recommended.

   https://github.com/cs3org/reva/issues/1758
   https://github.com/cs3org/reva/pull/1760

 * Enhancement #1747: Add checksum headers to tus preflight responses

   Added `checksum` to the header `Tus-Extension` and added the `Tus-Checksum-Algorithm`
   header.

   https://github.com/owncloud/ocis/issues/1747
   https://github.com/cs3org/reva/pull/1702

 * Enhancement #1685: Add share to update response

   After accepting or rejecting a share the API includes the updated share in the response.

   https://github.com/cs3org/reva/pull/1685
   https://github.com/cs3org/reva/pull/1724


