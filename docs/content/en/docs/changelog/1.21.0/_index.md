
---
title: "v1.21.0"
linkTitle: "v1.21.0"
weight: 40
description: >
  Changelog for Reva v1.21.0 (2022-12-07)
---

Changelog for reva 1.21.0 (2022-12-07)
=======================================

The following sections list the changes in reva 1.21.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3492: Fixes the DefaultQuotaBytes in EOS
 * Fix #3420: EOS grpc fixes
 * Fix #3501: Fix errors of public share provider according to cs3apis
 * Fix #3504: Fix RefreshLock method for cephfs storage driver
 * Enh #3502: Appproviders: pass other query parameters as Opaque
 * Enh #3028: Access directly auth registry rules map when getting provider
 * Enh #3197: Bring back multi-stage build to save on image size
 * Enh #3506: Migrate the buildAndPublishDocker job from Drone to GitHub Actions
 * Enh #3500: Migrate the BuildOnly job from Drone to GitHub Actions
 * Enh #3513: Migrate the testIntegration job from Drone to GitHub Actions
 * Enh #3494: Implemented folderurl for WOPI apps
 * Enh #3507: Get user preferred language
 * Enh #3530: Improve error logging in ocmd flow
 * Enh #3491: Implement rclone third-party copy push option
 * Enh #3508: Allow an user to set a preferred language

Details
-------

 * Bugfix #3492: Fixes the DefaultQuotaBytes in EOS

   We were setting the default logical quota to 1T, resulting on only 500GB available to the user.

   https://github.com/cs3org/reva/pull/3492

 * Bugfix #3420: EOS grpc fixes

   The shares and the applications were not working with the EOS grpc storage driver. This fixes
   both.

   https://github.com/cs3org/reva/pull/3420

 * Bugfix #3501: Fix errors of public share provider according to cs3apis

   All the errors returned by the public share provider where internal errors. Now this has been
   fixed and the returned errors are the one defined in the cs3apis.

   https://github.com/cs3org/reva/pull/3501

 * Bugfix #3504: Fix RefreshLock method for cephfs storage driver

   https://github.com/cs3org/reva/pull/3504

 * Enhancement #3502: Appproviders: pass other query parameters as Opaque

   This allows to send any other HTTP query parameter passed to /app/open to the underlying
   appprovider drivers via GRPC

   https://github.com/cs3org/reva/pull/3502

 * Enhancement #3028: Access directly auth registry rules map when getting provider

   https://github.com/cs3org/reva/pull/3028

 * Enhancement #3197: Bring back multi-stage build to save on image size

   - Use EOS 4.8.91 as base image - Bring back multi-stage build - Build revad on the eos 4.8.91 image
   due to missing dependency (`ld-musl-x86_64.so.1`, typical of alpine) - Copy the resulting
   revad from the builder container

   Resulting image size (unpacked on disk) is 2.59GB - eos-all:4.8.91 is 2.47GB - existing
   revad:latest-eos is 6.18GB

   https://github.com/cs3org/reva/pull/3197

 * Enhancement #3506: Migrate the buildAndPublishDocker job from Drone to GitHub Actions

   We've migrated the buildAndPublishDocker job from Drone to GitHub Actions workflow. We've
   updated the Golang version used to build the Docker images to go1.19. We've fixed the Cephfs
   storage module. We've improved the Makefile. We've refactored the build-docker workflow.

   https://github.com/cs3org/reva/pull/3506

 * Enhancement #3500: Migrate the BuildOnly job from Drone to GitHub Actions

   We've migrated the BuildOnly job from Drone to GitHub Actions workflow. The Workflow builds
   and Tests Reva, builds a Revad Docker Image and checks the license headers. The license header
   tools was removed since the goheader linter provides the same functionality.

   https://github.com/cs3org/reva/pull/3500

 * Enhancement #3513: Migrate the testIntegration job from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3513

 * Enhancement #3494: Implemented folderurl for WOPI apps

   The folderurl is now populated for WOPI apps, such that for owners and named shares it points to
   the containing folder, and for public links it points to the appropriate public link URL.

   On the way, functions to manipulate the user's scope and extract the eventual public link
   token(s) have been added, coauthored with @gmgigi96.

   https://github.com/cs3org/reva/pull/3494

 * Enhancement #3507: Get user preferred language

   The only way for an OCIS web user to change language was to set it into the browser settings. In the
   ocs user info response, a field `language` is added, to change their language in the UI,
   regardless of the browser settings.

   https://github.com/cs3org/reva/pull/3507

 * Enhancement #3530: Improve error logging in ocmd flow

   https://github.com/cs3org/reva/issues/3365
   https://github.com/cs3org/reva/pull/3530
   https://github.com/cs3org/reva/pull/3526
   https://github.com/cs3org/reva/pull/3419
   https://github.com/cs3org/reva/pull/3369

 * Enhancement #3491: Implement rclone third-party copy push option

   This enhancement gives the option to use third-party copy push with rclone between two
   different user accounts.

   https://github.com/cs3org/reva/pull/3491

 * Enhancement #3508: Allow an user to set a preferred language

   https://github.com/cs3org/reva/pull/3508


