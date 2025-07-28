
---
title: "v1.24.0"
linkTitle: "v1.24.0"
weight: 40
description: >
  Changelog for Reva v1.24.0 (2023-05-11)
---

Changelog for reva 1.24.0 (2023-05-11)
=======================================

The following sections list the changes in reva 1.24.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3805: Apps: fixed viewMode resolution
 * Fix #3771: Fix files sharing capabilities
 * Fix #3749: Fix persisting updates of received shares in json driver
 * Fix #3723: Fix revad docker images by enabling CGO
 * Fix #3765: Fix create version folder in EOS driver
 * Fix #3786: Fix listing directory for a read-only shares for EOS storage driver
 * Fix #3787: Fix application flag for EOS binary
 * Fix #3780: Fix Makefile error on Ubuntu
 * Fix #3873: Fix parsing of grappa response
 * Fix #3794: Fix unshare for EOS storage driver
 * Fix #3838: Fix upload in a single file share for lightweight accounts
 * Fix #3878: Fix creator/initiator in public and user shares
 * Fix #3813: Fix propfind URL for OCM shares
 * Fix #3770: Fixed default protocol on ocm-share-create
 * Fix #3852: Pass remote share id and shared secret in OCM call
 * Fix #3859: Fix inconsistency between data transfer protocol naming
 * Enh #3847: Update data transfers for current OCM shares implementation
 * Enh #3869: Datatx tutorial
 * Enh #3762: Denial and Resharing Default capabilities
 * Enh #3717: Disable sharing on low level paths
 * Enh #3766: Download file revisions
 * Enh #3733: Add support for static linking
 * Enh #3778: Add support to tag eos traffic
 * Enh #3868: Implement historical way of constructing OCM WebDAV URL
 * Enh #3719: Skip computing groups when fetching all groups from grappa
 * Enh #3783: Updated OCM tutorial
 * Enh #3750: New metadata flags
 * Enh #3839: Support multiple issuer in OIDC auth driver
 * Enh #3772: New OCM discovery endpoint
 * Enh #3619: Tests for invitation manager SQL driver
 * Enh #3757: Support OCM v1.0 schema
 * Enh #3695: Create OCM share from sciencemesh service
 * Enh #3722: List only valid OCM tokens
 * Enh #3821: Revamp user/group drivers and fix user type
 * Enh #3724: Send invitation link from mesh directory
 * Enh #3824: Serverless Services

Details
-------

 * Bugfix #3805: Apps: fixed viewMode resolution

   Currently, the viewMode passed on /app/open is taken without validating the actual user's
   permissions. This PR fixes this.

   https://github.com/cs3org/reva/pull/3805

 * Bugfix #3771: Fix files sharing capabilities

   A bug was preventing setting some capabilities (ResharingDefault and DenyAccess) for files
   sharing from the configuration file

   https://github.com/cs3org/reva/pull/3771

 * Bugfix #3749: Fix persisting updates of received shares in json driver

   https://github.com/cs3org/reva/pull/3749

 * Bugfix #3723: Fix revad docker images by enabling CGO

   https://github.com/cs3org/reva/pull/3723

 * Bugfix #3765: Fix create version folder in EOS driver

   In a read only share, a stat could fail, beacause the EOS storage driver was not able to create the
   version folder for a file in case this did not exist. This fixes this bug impersonating the owner
   of the file when creating the version folder.

   https://github.com/cs3org/reva/pull/3765

 * Bugfix #3786: Fix listing directory for a read-only shares for EOS storage driver

   In a read-only share, while listing a folder, for resources not having a version folder, the
   returned resource id was wrongly the one of the original file, instead of the version folder.
   This behavior has been fixed, where the version folder is always created on behalf of the
   resource owner.

   https://github.com/cs3org/reva/pull/3786

 * Bugfix #3787: Fix application flag for EOS binary

   https://github.com/cs3org/reva/pull/3787

 * Bugfix #3780: Fix Makefile error on Ubuntu

   I've fixed Makefile using sh which is defaulted to dash in ubuntu, dash doesn't support `[[ ...
   ]]` syntax and Makefile would throw `/bin/sh: 1: [[: not found` errors.

   https://github.com/cs3org/reva/issues/3773
   https://github.com/cs3org/reva/pull/3780

 * Bugfix #3873: Fix parsing of grappa response

   https://github.com/cs3org/reva/pull/3873

 * Bugfix #3794: Fix unshare for EOS storage driver

   In the EOS storage driver, the remove acl operation was a no-op. After removing a share, the
   recipient of the share was still able to operate on the shared resource. Now this has been fixed,
   removing correctly the ACL from the shared resource.

   https://github.com/cs3org/reva/pull/3794

 * Bugfix #3838: Fix upload in a single file share for lightweight accounts

   https://github.com/cs3org/reva/pull/3838

 * Bugfix #3878: Fix creator/initiator in public and user shares

   https://github.com/cs3org/reva/pull/3878

 * Bugfix #3813: Fix propfind URL for OCM shares

   https://github.com/cs3org/reva/issues/3810
   https://github.com/cs3org/reva/pull/3813

 * Bugfix #3770: Fixed default protocol on ocm-share-create

   https://github.com/cs3org/reva/pull/3770

 * Bugfix #3852: Pass remote share id and shared secret in OCM call

   https://github.com/cs3org/reva/pull/3852

 * Bugfix #3859: Fix inconsistency between data transfer protocol naming

   https://github.com/cs3org/reva/issues/3858
   https://github.com/cs3org/reva/pull/3859

 * Enhancement #3847: Update data transfers for current OCM shares implementation

   https://github.com/cs3org/reva/issues/3846
   https://github.com/cs3org/reva/pull/3847

 * Enhancement #3869: Datatx tutorial

   https://github.com/cs3org/reva/issues/3864
   https://github.com/cs3org/reva/pull/3869

 * Enhancement #3762: Denial and Resharing Default capabilities

   https://github.com/cs3org/reva/pull/3762

 * Enhancement #3717: Disable sharing on low level paths

   Sharing can be disable in the user share provider for some paths, but the storage provider was
   still sending the sharing permissions for those paths. This adds a config option in the storage
   provider, `minimum_allowed_path_level_for_share`, to disable sharing permissions for
   resources up to a defined path level.

   https://github.com/cs3org/reva/pull/3717

 * Enhancement #3766: Download file revisions

   Currently it is only possible to restore a file version, replacing the actual file with the
   selected version. This allows an user to download a version file, without touching/replacing
   the last version of the file

   https://github.com/cs3org/reva/pull/3766

 * Enhancement #3733: Add support for static linking

   We've added support for compiling reva with static linking enabled. It's possible to do so with
   the `STATIC` flag: `make revad STATIC=true`

   https://github.com/cs3org/reva/pull/3733

 * Enhancement #3778: Add support to tag eos traffic

   We've added support to tag eos traffic

   https://github.com/cs3org/reva/pull/3778

 * Enhancement #3868: Implement historical way of constructing OCM WebDAV URL

   Expose the expected WebDAV endpoint for OCM by OC10 and Nextcloud as described in
   https://github.com/cs3org/OCM-API/issues/70#issuecomment-1538551138 to allow reva
   providers to participate to mesh.

   https://github.com/cs3org/reva/issues/3855
   https://github.com/cs3org/reva/pull/3868

 * Enhancement #3719: Skip computing groups when fetching all groups from grappa

   https://github.com/cs3org/reva/pull/3719

 * Enhancement #3783: Updated OCM tutorial

   The OCM tutorial in the doc was missing the example on how to access the received resources. Now
   the tutorial contains all the steps to access a received resource using the WebDAV protocol.

   https://github.com/cs3org/reva/pull/3783

 * Enhancement #3750: New metadata flags

   Several new flags, like site infrastructure and service status, are now gathered and exposed
   by Mentix.

   https://github.com/cs3org/reva/pull/3750

 * Enhancement #3839: Support multiple issuer in OIDC auth driver

   The OIDC auth driver supports now multiple issuers. Users of external providers are then
   mapped to a local user by a mapping files. Only the main issuer (defined in the config with
   `issuer`) and the ones defined in the mapping are allowed for the verification of the OIDC
   token.

   https://github.com/cs3org/reva/pull/3839

 * Enhancement #3772: New OCM discovery endpoint

   This PR implements the new OCM v1.1 specifications for the /ocm-provider endpoint.

   https://github.com/cs3org/reva/pull/3772

 * Enhancement #3619: Tests for invitation manager SQL driver

   https://github.com/cs3org/reva/pull/3619

 * Enhancement #3757: Support OCM v1.0 schema

   Following cs3org/cs3apis#206, we add the fields to ensure backwards compatibility with OCM
   v1.0. However, if the `protocol.options` undocumented object is not empty, we bail out for
   now. Supporting interoperability with OCM v1.0 implementations (notably Nextcloud 25) may
   come in the future if the undocumented options are fully reverse engineered. This is reflected
   in the unit tests as well.

   Also, added viewMode to webapp protocol options (cs3org/cs3apis#207) and adapted all SQL
   code and unit tests.

   https://github.com/cs3org/reva/pull/3757

 * Enhancement #3695: Create OCM share from sciencemesh service

   https://github.com/pondersource/sciencemesh-php/issues/166
   https://github.com/cs3org/reva/pull/3695

 * Enhancement #3722: List only valid OCM tokens

   https://github.com/cs3org/reva/pull/3722

 * Enhancement #3821: Revamp user/group drivers and fix user type

   For lightweight accounts

   * Fix the user type for lightweight accounts, using the source field to differentiate between a
   primary and lw account * Remove all the code with manual parsing of the json returned by the CERN
   provider * Introduce pagination for `GetMembers` method in the group driver * Reduced network
   transfer size by requesting only needed fields for `GetMembers` method

   https://github.com/cs3org/reva/pull/3821

 * Enhancement #3724: Send invitation link from mesh directory

   When generating and listing OCM tokens

   To enhance user expirience, instead of only sending the token, we send directly the URL for
   accepting the invitation workflow.

   https://github.com/cs3org/reva/pull/3724

 * Enhancement #3824: Serverless Services

   New type of service (along with http and grpc) which does not have a listening server. Useful for
   the notifications service and others in the future.

   https://github.com/cs3org/reva/pull/3824


