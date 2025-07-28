
---
title: "v1.29.0"
linkTitle: "v1.29.0"
weight: 40
description: >
  Changelog for Reva v1.29.0 (2025-01-07)
---

Changelog for reva 1.29.0 (2025-01-07)
=======================================

The following sections list the changes in reva 1.29.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4898: Make ACL operations work over gRPC
 * Fix #4667: Fixed permission mapping to EOS ACLs
 * Fix #4520: Do not use version folders for xattrs in EOS
 * Fix #4599: Auth: increase verbosity of oidc parsing errors
 * Fix #5006: Blocking reva on listSharedWithMe
 * Fix #4557: Fix ceph build
 * Fix #5017: No empty favs attr
 * Fix #4620: Fix ulimits for EOS container deployment
 * Fix #5015: Fixed error reporting in the EOS gRPC client
 * Fix #4931: Fixed tree metadata following fix in EOS
 * Fix #4930: Make removal of favourites work
 * Fix #4574: Fix notifications
 * Fix #4790: Ocm: fixed domain not having a protocol scheme
 * Fix #4849: Drop assumptions about user types when dealing with shares
 * Fix #4894: No certs in EOS HTTP client
 * Fix #4810: Simplified error handling
 * Fix #4973: Handle parsing of favs over gRPC
 * Fix #4901: Broken PROPFIND perms on gRPC
 * Fix #4907: Public links: return error when owner could not be resolved
 * Fix #4591: Eos: fixed error reporting for too large recycle bin listing
 * Fix #4896: Fix nilpointer error in RollbackToVersion
 * Fix #4905: PurgeDate in ListDeletedEntries was ignored
 * Fix #4939: Revert 'make home layout configurable'
 * Enh #5028: Handle empty EOS traces
 * Enh #4911: Cephfs refactoring + make home layout configurable
 * Enh #4937: @labkode steps down as project owner
 * Enh #4579: Remove domain-specific code to other repos
 * Enh #4824: Refactor Ceph code
 * Enh #4797: Refactor CI jobs and bump to latest deps
 * Enh #4934: Access to EOS via tokens over gRPC
 * Enh #4870: Only load X509 on https
 * Enh #5014: Log app when creating EOS gRPC requests
 * Enh #4892: Do not read eos user ACLs any longer
 * Enh #4720: Differentiate quota for user types in EOS
 * Enh #4863: Favourites for eos/grpc
 * Enh #5013: Updated dependencies + moved to go 1.22
 * Enh #4514: Pass lock holder metadata on uploads
 * Enh #4970: Improved logging on createHome
 * Enh #4984: Drop shadow namespaces
 * Enh #4670: Ocm: support bearer token access
 * Enh #4977: Do not use root on EOS

Details
-------

 * Bugfix #4898: Make ACL operations work over gRPC

   This change solves two issues: * AddACL would fail, because the current implementation of
   AddACL in the EOS gRPC client always sets msg.Recursive = true. This causes issues on the EOS
   side, because it will try running a recursive find on a file, which fails. * RemoveACL would
   fail, because it tried matching ACL rules with a uid to ACL rules with a username. This PR changes
   this approach to use an approach similar to what is used in the binary client: just set the rule
   that you want to have deleted with no permissions.

   https://github.com/cs3org/reva/pull/4898

 * Bugfix #4667: Fixed permission mapping to EOS ACLs

   This is to remove "m" and "q" flags in EOS ACLs for regular write shares (no re-sharing).

   https://github.com/cs3org/reva/pull/4667

 * Bugfix #4520: Do not use version folders for xattrs in EOS

   This was a workaround needed some time ago. We revert now to the standard behavior, xattrs are
   stored on the files.

   https://github.com/cs3org/reva/pull/4520

 * Bugfix #4599: Auth: increase verbosity of oidc parsing errors

   This is to help further debugging of auth issues. An unrelated error reporting was also fixed.

   https://github.com/cs3org/reva/pull/4599

 * Bugfix #5006: Blocking reva on listSharedWithMe

   `listSharesWithMe` blocked a reva thread in the case that one of the shares was not resolvable.
   This has now been fixed

   https://github.com/cs3org/reva/pull/5006

 * Bugfix #4557: Fix ceph build

   https://github.com/cs3org/reva/pull/4557

 * Bugfix #5017: No empty favs attr

   See issue #5016: we now unset the favs attr if no more favs are set

   https://github.com/cs3org/reva/pull/5017

 * Bugfix #4620: Fix ulimits for EOS container deployment

   https://github.com/cs3org/reva/pull/4620

 * Bugfix #5015: Fixed error reporting in the EOS gRPC client

   This in particular fixes the lock-related errors

   https://github.com/cs3org/reva/pull/5015

 * Bugfix #4931: Fixed tree metadata following fix in EOS

   The treecount is now populated from the EOS response.

   https://github.com/cs3org/reva/pull/4931

 * Bugfix #4930: Make removal of favourites work

   Currently, removing a folder from your favourites is broken, because the handleFavAttr
   method is only called in SetAttr, not in UnsetAttr. This change fixes this.

   https://github.com/cs3org/reva/pull/4930

 * Bugfix #4574: Fix notifications

   https://github.com/cs3org/reva/pull/4574

 * Bugfix #4790: Ocm: fixed domain not having a protocol scheme

   This PR fixes a bug in the OCM open driver that causes it to be unable to probe OCM services at the
   remote server due to the domain having an unsupported protocol scheme. in this case domain
   doesn't have a scheme and the changes in this PR add a scheme to the domain before doing the probe.

   https://github.com/cs3org/reva/pull/4790

 * Bugfix #4849: Drop assumptions about user types when dealing with shares

   We may have external accounts with regular usernames (and with null uid), therefore the
   current logic to heuristically infer the user type from a grantee's username is broken. This PR
   removes those heuristics and requires the upper level to resolve the user type.

   https://github.com/cs3org/reva/pull/4849

 * Bugfix #4894: No certs in EOS HTTP client

   Omit HTTPS cert in EOS HTTP Client, as this causes authentication issues on EOS < 5.2.28. When
   EOS receives a certificate, it will look for this cert in the gridmap file. If it is not found
   there, the whole authn flow is aborted and the user is mapped to nobody.

   https://github.com/cs3org/reva/pull/4894

 * Bugfix #4810: Simplified error handling

   Minor rewording and simplification, following cs3org/OCM-API#90 and cs3org/OCM-API#91

   https://github.com/cs3org/reva/pull/4810

 * Bugfix #4973: Handle parsing of favs over gRPC

   To store user favorites, the key `user.http://owncloud.org/ns/favorite` maps to a list of
   users, in the format `u:username=1`. Right now, extracting the "correct" user doesn't happen
   in gRPC, while it is implemented in the EOS binary client. This feature has now been moved to the
   higher-level call in eosfs.

   https://github.com/cs3org/reva/pull/4973

 * Bugfix #4901: Broken PROPFIND perms on gRPC

   When using the EOS gRPC stack, the permissions returned by PROPFIND on a folder in a project were
   erroneous because ACL permissions were being ignored. This stems from a bug in
   grpcMDResponseToFileInfo, where the SysACL attribute of the FileInfo struct was not being
   populated.

   https://github.com/cs3org/reva/pull/4901
   see:

 * Bugfix #4907: Public links: return error when owner could not be resolved

   https://github.com/cs3org/reva/pull/4907

 * Bugfix #4591: Eos: fixed error reporting for too large recycle bin listing

   EOS returns E2BIG, which internally gets converted to PermissionDenied and has to be properly
   handled in this case.

   https://github.com/cs3org/reva/pull/4591

 * Bugfix #4896: Fix nilpointer error in RollbackToVersion

   https://github.com/cs3org/reva/pull/4896

 * Bugfix #4905: PurgeDate in ListDeletedEntries was ignored

   The date range that can be passed to ListDeletedEntries was not taken into account due to a bug in
   reva: the Purgedate argument was set, which only works for PURGE requests, and not for LIST
   requests. Instead, the Listflag argument must be used. Additionally, there was a bug in the
   loop that is used to iterate over all days in the date range.

   https://github.com/cs3org/reva/pull/4905

 * Bugfix #4939: Revert 'make home layout configurable'

   Partial revert of #4911, to be re-added after more testing and configuration validation. The
   eoshome vs eos storage drivers are to be adapted.

   https://github.com/cs3org/reva/pull/4939

 * Enhancement #5028: Handle empty EOS traces

   https://github.com/cs3org/reva/pull/5028

 * Enhancement #4911: Cephfs refactoring + make home layout configurable

   https://github.com/cs3org/reva/pull/4911

 * Enhancement #4937: @labkode steps down as project owner

   Hugo (@labkode) steps down as project owner of Reva.

   https://github.com/cs3org/reva/pull/4937

 * Enhancement #4579: Remove domain-specific code to other repos

   https://github.com/cs3org/reva/pull/4579

 * Enhancement #4824: Refactor Ceph code

   https://github.com/cs3org/reva/pull/4824

 * Enhancement #4797: Refactor CI jobs and bump to latest deps

   https://github.com/cs3org/reva/pull/4797

 * Enhancement #4934: Access to EOS via tokens over gRPC

   As a guest account, accessing a file shared with you relies on a token that is generated on behalf
   of the resource owner. This method, GenerateToken, has now been implemented in the EOS gRPC
   client. Additionally, the HTTP client now takes tokens into account.

   https://github.com/cs3org/reva/pull/4934

 * Enhancement #4870: Only load X509 on https

   Currently, the EOS HTTP Client always tries to read an X509 key pair from the file system (by
   default, from /etc/grid-security/host{key,cert}.pem). This makes it harder to write unit
   tests, as these fail when this key pair is not on the file system (which is the case for the test
   pipeline as well).

   This PR introduces a fix for this problem, by only loading the X509 key pair if the scheme of the
   EOS endpoint is https. Unit tests can then create a mock HTTP endpoint, which will not trigger
   the loading of the key pair.

   https://github.com/cs3org/reva/pull/4870

 * Enhancement #5014: Log app when creating EOS gRPC requests

   https://github.com/cs3org/reva/pull/5014

 * Enhancement #4892: Do not read eos user ACLs any longer

   This PR drops the compatibility code to read eos user ACLs in the eos binary client, and aligns it
   to the GRPC client.

   https://github.com/cs3org/reva/pull/4892

 * Enhancement #4720: Differentiate quota for user types in EOS

   We now assign a different initial quota to users depending on their type, whether PRIMARY or
   not.

   https://github.com/cs3org/reva/pull/4720

 * Enhancement #4863: Favourites for eos/grpc

   https://github.com/cs3org/reva/pull/4863

 * Enhancement #5013: Updated dependencies + moved to go 1.22

   https://github.com/cs3org/reva/pull/5013

 * Enhancement #4514: Pass lock holder metadata on uploads

   We now pass relevant metadata (lock id and lock holder) downstream on uploads, and handle the
   case of conflicts due to lock mismatch.

   https://github.com/cs3org/reva/pull/4514

 * Enhancement #4970: Improved logging on createHome

   https://github.com/cs3org/reva/pull/4970

 * Enhancement #4984: Drop shadow namespaces

   This comes as part of the effort to operate EOS without being root, see
   https://github.com/cs3org/reva/pull/4977

   In this PR the post-home creation hook (and corresponding flag) is replaced by a
   create_home_hook, and the following configuration parameters are suppressed:

   Shadow_namespace share_folder default_quota_bytes default_secondary_quota_bytes
   default_quota_files uploads_namespace (unused)

   https://github.com/cs3org/reva/pull/4984

 * Enhancement #4670: Ocm: support bearer token access

   This PR adds support for accessing remote OCM 1.1 shares via bearer token, as opposed to having
   the shared secret in the URL only. In addition, the OCM client package is now part of the OCMD
   server package, and the Discover methods have been all consolidated in one place.

   https://github.com/cs3org/reva/pull/4670

 * Enhancement #4977: Do not use root on EOS

   Currently, the EOS drivers use root authentication for many different operations. This has
   now been changed to use one of the following: * cbox, which is a sudo'er * daemon, for read-only
   operations * the user himselft

   Note that home creation is excluded here as this will be tackled in a different PR.

   https://github.com/cs3org/reva/pull/4977/


