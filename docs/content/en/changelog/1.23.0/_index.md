
---
title: "v1.23.0"
linkTitle: "v1.23.0"
weight: 40
description: >
  Changelog for Reva v1.23.0 (2023-03-09)
---

Changelog for reva 1.23.0 (2023-03-09)
=======================================

The following sections list the changes in reva 1.23.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3621: Use 2700 as permission when creating EOS home folder
 * Fix #3551: Fixes implementation omission of #3526
 * Fix #3706: Fix revad-eos docker image which was failing to build
 * Fix #3626: Fix open in app for lightweight accounts
 * Fix #3613: Use subject from oidc userinfo when quering the user provider
 * Fix #3633: Fix litmus and acceptance tests in GitHub Actions
 * Fix #3694: Updated public links URLs and users' display names in WOPI apps
 * Chg #3553: Rename PullTransfer to CreateTransfer
 * Enh #3584: Bump the Copyright date to 2023
 * Enh #3640: Migrate acceptance tests from Drone to GitHub Actions
 * Enh #3629: Use cs3org/behat:latest docker image for tests
 * Enh #3608: Add Golang test coverage report for Codacy
 * Enh #3599: Add latest tag to revad Docker image with GitHub Actions
 * Enh #3713: Streamline EOS SSS and UNIX modes
 * Enh #3566: Migrate the litmusOcisSpacesDav test from Drone to GitHub Actions
 * Enh #3712: Improve Docker build speed and Docker Compose test speed
 * Enh #3630: Migrate the virtualViews test from Drone to GitHub Actions
 * Enh #3675: Cleanup unused configs in OCM HTTP service
 * Enh #3692: Create and list OCM shares in OCS layer
 * Enh #3666: Search OCM accepted users
 * Enh #3665: List valid OCM invite tokens
 * Enh #3617: SQL driver for OCM invitation manager
 * Enh #3667: List OCM providers
 * Enh #3668: Expose OCM received shares as a local mount
 * Enh #3683: Remote open in app in OCM
 * Enh #3654: SQL driver for OCM shares
 * Enh #3646: Update OCM shares to last version of CS3APIs
 * Enh #3687: Specify recipient as a query param when sending OCM token by email
 * Enh #3691: Add OCM scope and webdav endpoint
 * Enh #3611: Revamp OCM invitation workflow
 * Enh #3703: Bump reva(d) base image to alpine 3.17

Details
-------

 * Bugfix #3621: Use 2700 as permission when creating EOS home folder

   https://github.com/cs3org/reva/pull/3621

 * Bugfix #3551: Fixes implementation omission of #3526

   In #3526 a new value format of the owner parameter of the ocm share request was introduced. This
   change was not implemented in the json driver. This change fixes that.

   https://github.com/cs3org/reva/pull/3551

 * Bugfix #3706: Fix revad-eos docker image which was failing to build

   https://github.com/cs3org/reva/pull/3706

 * Bugfix #3626: Fix open in app for lightweight accounts

   https://github.com/cs3org/reva/pull/3626

 * Bugfix #3613: Use subject from oidc userinfo when quering the user provider

   https://github.com/cs3org/reva/pull/3613

 * Bugfix #3633: Fix litmus and acceptance tests in GitHub Actions

   https://github.com/cs3org/reva/pull/3633

 * Bugfix #3694: Updated public links URLs and users' display names in WOPI apps

   Public links have changed in the frontend and are reflected in folderurl query parameter.
   Additionally, OCM shares are supported for the folderurl and OCM users are decorated with
   their ID provider.

   https://github.com/cs3org/reva/pull/3694

 * Change #3553: Rename PullTransfer to CreateTransfer

   This change implements a CS3APIs name change in the datatx module (PullTransfer to
   CreateTransfer)

   https://github.com/cs3org/reva/pull/3553

 * Enhancement #3584: Bump the Copyright date to 2023

   https://github.com/cs3org/reva/pull/3584

 * Enhancement #3640: Migrate acceptance tests from Drone to GitHub Actions

   Migrate ocisIntegrationTests and s3ngIntegrationTests to GitHub Actions

   https://github.com/cs3org/reva/pull/3640

 * Enhancement #3629: Use cs3org/behat:latest docker image for tests

   https://github.com/cs3org/reva/pull/3629

 * Enhancement #3608: Add Golang test coverage report for Codacy

   https://github.com/cs3org/reva/pull/3608

 * Enhancement #3599: Add latest tag to revad Docker image with GitHub Actions

   https://github.com/cs3org/reva/pull/3599

 * Enhancement #3713: Streamline EOS SSS and UNIX modes

   https://github.com/cs3org/reva/pull/3713

 * Enhancement #3566: Migrate the litmusOcisSpacesDav test from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3566

 * Enhancement #3712: Improve Docker build speed and Docker Compose test speed

   https://github.com/cs3org/reva/pull/3712

 * Enhancement #3630: Migrate the virtualViews test from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3630

 * Enhancement #3675: Cleanup unused configs in OCM HTTP service

   https://github.com/cs3org/reva/pull/3675

 * Enhancement #3692: Create and list OCM shares in OCS layer

   https://github.com/cs3org/reva/pull/3692

 * Enhancement #3666: Search OCM accepted users

   Adds the prefix `sm:` to the FindUser endpoint, to filter only the OCM accepted users.

   https://github.com/cs3org/reva/pull/3666

 * Enhancement #3665: List valid OCM invite tokens

   Adds the endpoint `/list-invite` in the sciencemesh service, to get the list of valid OCM
   invite tokens.

   https://github.com/cs3org/reva/pull/3665
   https://github.com/cs3org/cs3apis/pull/201

 * Enhancement #3617: SQL driver for OCM invitation manager

   https://github.com/cs3org/reva/pull/3617

 * Enhancement #3667: List OCM providers

   Adds the endpoint `/list-providers` in the sciencemesh service, to get a filtered list of the
   OCM providers. The filter can be specified with the `search` query parameters, and filters by
   domain and full name of the provider.

   https://github.com/cs3org/reva/pull/3667

 * Enhancement #3668: Expose OCM received shares as a local mount

   https://github.com/cs3org/reva/pull/3668

 * Enhancement #3683: Remote open in app in OCM

   https://github.com/cs3org/reva/pull/3683

 * Enhancement #3654: SQL driver for OCM shares

   https://github.com/cs3org/reva/pull/3654

 * Enhancement #3646: Update OCM shares to last version of CS3APIs

   https://github.com/cs3org/reva/pull/3646
   https://github.com/cs3org/cs3apis/pull/199

 * Enhancement #3687: Specify recipient as a query param when sending OCM token by email

   Before the email recipient when sending the OCM token was specified as a form parameter. Now as a
   query parameter, as some clients does not allow in a GET request to set form values. It also add
   the possibility to specify a template for the subject and the body for the token email.

   https://github.com/cs3org/reva/pull/3687

 * Enhancement #3691: Add OCM scope and webdav endpoint

   Adds the OCM scope and the ocmshares authentication, to authenticate the federated user to use
   the OCM shared resources. It also adds the (unprotected) webdav endpoint used to interact with
   the shared resources.

   https://github.com/cs3org/reva/issues/2739
   https://github.com/cs3org/reva/pull/3691

 * Enhancement #3611: Revamp OCM invitation workflow

   https://github.com/cs3org/reva/issues/3540
   https://github.com/cs3org/reva/pull/3611

 * Enhancement #3703: Bump reva(d) base image to alpine 3.17

   Prevents several vulnerabilities from the base image itself:
   https://artifacthub.io/packages/helm/cs3org/revad?modal=security-report

   https://github.com/cs3org/reva/pull/3703


