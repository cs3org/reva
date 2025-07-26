
---
title: "v1.20.0"
linkTitle: "v1.20.0"
weight: 40
description: >
  Changelog for Reva v1.20.0 (2022-11-24)
---

Changelog for reva 1.20.0 (2022-11-24)
=======================================

The following sections list the changes in reva 1.20.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Sec #3316: Mitigate XSS
 * Fix #3455: Fixes panic in case of empty configuration
 * Fix #3311: Remove FIXME
 * Fix #3396: Fix the Ceph Docker image repository URL
 * Fix #3055: Fix quota for LW accounts
 * Fix #3361: Use custom reva logger in ocs
 * Fix #3344: Fix quota percentage
 * Fix #2979: Removed unused datatx code
 * Fix #2973: Fix datatxtarget uri when prefix is used
 * Fix #3319: Fix oidc provider crash when custom claims are provided
 * Fix #3481: OIDC: resolve users with no uid/gid by username
 * Fix #3055: Get user from user provider in oidc driver
 * Fix #3053: Temporary read user acl instead of sys acl
 * Enh #3401: Make WOPI bridged apps (CodiMD) configuration non hard-coded
 * Enh #3402: Block users
 * Enh #3098: App provider http endpoint uses Form instead of Query
 * Enh #3116: Implementation of cback storage driver for REVA
 * Enh #3422: Migrate Codacy from Drone to Codacy/GitHub integration
 * Enh #3412: Migrate Fossa from Drone to Github Integration
 * Enh #3367: Update go version
 * Enh #3467: Enable gocritic linter in golangci-lint and solve issues
 * Enh #3463: Enable gofmt linter in golangci-lint and apply gofmt
 * Enh #3471: Enable goimports and usestdlibvars in golangci-lint
 * Enh #3466: Migrate golangci-lint from Drone to GitHub Actions
 * Enh #3465: Enable revive linter in golangci-lint and solve issues
 * Enh #3487: Enable staticcheck linter in golangci-lint and solve issues
 * Enh #3475: Enable the style linters
 * Enh #3070: Allow http service to expose prefixes containing /
 * Enh #2986: Better display name in apps for all user types
 * Enh #3303: Added support for configuring language locales in apps
 * Enh #3348: Revamp lightweigth accounts
 * Enh #3304: Add http service to send email for shares
 * Enh #3072: Mesh meta data operators
 * Enh #3313: Fix content-type for OCM sharing
 * Enh #3234: Add post create home hook for eos storage driver
 * Enh #3347: Implemented PROPFIND with 0 depth
 * Enh #3056: Add public share auth provider
 * Enh #3305: Add description to public link
 * Enh #3163: Add support for quicklinks for public shares
 * Enh #3289: Make Refresh Lock operation WOPI compliant
 * Enh #3315: Accept reva token as a bearer authentication
 * Enh #3438: Sanitize non-utf8 characters in xattr values in EOS
 * Enh #3221: Site Accounts improvements
 * Enh #3404: Site accounts & Mentix updates
 * Enh #3424: Expire tokens on sunday
 * Enh #2986: Use email as display name for external users opening WOPI apps

Details
-------

 * Security #3316: Mitigate XSS

   We've mitigated an XSS vulnerability resulting from unescaped HTTP responses containing
   user-provided values in pkg/siteacc/siteacc.go and
   internal/http/services/ocmd/invites.go. This patch uses html.EscapeString to escape the
   user-provided values in the HTTP responses of pkg/siteacc/siteacc.go and
   internal/http/services/ocmd/invites.go.

   https://github.com/cs3org/reva/pull/3316

 * Bugfix #3455: Fixes panic in case of empty configuration

   Makes sure the config map is allocated prior to setting it

   https://github.com/cs3org/reva/pull/3455

 * Bugfix #3311: Remove FIXME

   Issue https://github.com/cs3org/reva/issues/2402 is closed.

   https://github.com/cs3org/reva/pull/3311

 * Bugfix #3396: Fix the Ceph Docker image repository URL

   https://github.com/cs3org/reva/pull/3396

 * Bugfix #3055: Fix quota for LW accounts

   LW accounts do not have quota assigned.

   https://github.com/cs3org/reva/pull/3055

 * Bugfix #3361: Use custom reva logger in ocs

   https://github.com/cs3org/reva/pull/3361

 * Bugfix #3344: Fix quota percentage

   https://github.com/cs3org/reva/pull/3344

 * Bugfix #2979: Removed unused datatx code

   An OCM reference is not created for a data transfer type share.

   https://github.com/cs3org/reva/pull/2979

 * Bugfix #2973: Fix datatxtarget uri when prefix is used

   When a webdav prefix is used it appears in both host and name parameter of the target uri for data
   transfer. This PR fixes that.

   https://github.com/cs3org/reva/pull/2973

 * Bugfix #3319: Fix oidc provider crash when custom claims are provided

   https://github.com/cs3org/reva/pull/3319

 * Bugfix #3481: OIDC: resolve users with no uid/gid by username

   Previously we resolved such users (so called "lightweight" or "external" accounts in the CERN
   realm) by email, but it turns out that the same email may have multiple accounts associated to
   it.

   Therefore we now resolve them by username, that is the upn, which is unique.

   https://github.com/cs3org/reva/pull/3481

 * Bugfix #3055: Get user from user provider in oidc driver

   For oidc providers that only respond with standard claims, use the user provider to get the
   user.

   https://github.com/cs3org/reva/pull/3055

 * Bugfix #3053: Temporary read user acl instead of sys acl

   We read the user acl in EOS until the migration of all user acls to sys acls are done

   https://github.com/cs3org/reva/pull/3053

 * Enhancement #3401: Make WOPI bridged apps (CodiMD) configuration non hard-coded

   The configuration of the custom mimetypes has been moved to the AppProvider, and the given
   mimetypes are used to configure bridged apps by sharing the corresponding config item to the
   drivers.

   https://github.com/cs3org/reva/pull/3401

 * Enhancement #3402: Block users

   Allows an operator to set a list of users that are banned for every operation in reva.

   https://github.com/cs3org/reva/pull/3402

 * Enhancement #3098: App provider http endpoint uses Form instead of Query

   We've improved the http endpoint now uses the Form instead of Query to also support
   `application/x-www-form-urlencoded` parameters on the app provider http endpoint.

   https://github.com/cs3org/reva/pull/3098
   https://github.com/cs3org/reva/pull/3101

 * Enhancement #3116: Implementation of cback storage driver for REVA

   This is a read only fs interface.

   https://github.com/cs3org/reva/pull/3116

 * Enhancement #3422: Migrate Codacy from Drone to Codacy/GitHub integration

   https://github.com/cs3org/reva/pull/3422

 * Enhancement #3412: Migrate Fossa from Drone to Github Integration

   https://github.com/cs3org/reva/pull/3412

 * Enhancement #3367: Update go version

   Update go version to 1.19 in go.mod

   https://github.com/cs3org/reva/pull/3367

 * Enhancement #3467: Enable gocritic linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3467

 * Enhancement #3463: Enable gofmt linter in golangci-lint and apply gofmt

   https://github.com/cs3org/reva/pull/3463

 * Enhancement #3471: Enable goimports and usestdlibvars in golangci-lint

   We've enabled the goimports and usestdlibvars linters in golangci-lint and solved the
   related issues.

   https://github.com/cs3org/reva/pull/3471

 * Enhancement #3466: Migrate golangci-lint from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3466

 * Enhancement #3465: Enable revive linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3465

 * Enhancement #3487: Enable staticcheck linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3487

 * Enhancement #3475: Enable the style linters

   We've enabled the stylecheck, whitespace, dupword, godot and dogsled linters in
   golangci-lint and solved the related issues.

   https://github.com/cs3org/reva/pull/3475

 * Enhancement #3070: Allow http service to expose prefixes containing /

   https://github.com/cs3org/reva/pull/3070

 * Enhancement #2986: Better display name in apps for all user types

   This includes a `FirstName FamilyName (domain)` format for non-primary accounts, and a
   sanitization of the email address claim for such non-primary accounts.

   https://github.com/cs3org/reva/pull/2986
   https://github.com/cs3org/reva/pull/3280

 * Enhancement #3303: Added support for configuring language locales in apps

   This is a partial backport from edge: we introduce a language option in the appprovider, which
   if set is passed as appropriate parameter to the external apps in order to force a given
   localization. In particular, for Microsoft Office 365 the DC_LLCC option is set as well. The
   default behavior is unset, where apps try and resolve the localization from the browser
   headers.

   https://github.com/cs3org/reva/pull/3303

 * Enhancement #3348: Revamp lightweigth accounts

   Re-implements the lighweight account scope check, making it more efficient. Also, the ACLs
   for the EOS storage driver for the lw accounts are set atomically.

   https://github.com/cs3org/reva/pull/3348

 * Enhancement #3304: Add http service to send email for shares

   https://github.com/cs3org/reva/pull/3304

 * Enhancement #3072: Mesh meta data operators

   To better support sites that run multiple instances, the meta data have been extended to
   include a new hierarchy layer called 'operators'. This PR brings all necessary changes in the
   Mentix and site accounts services.

   https://github.com/cs3org/reva/pull/3072

 * Enhancement #3313: Fix content-type for OCM sharing

   This fix change the content type to just "application/json"

   https://github.com/cs3org/reva/pull/3313

 * Enhancement #3234: Add post create home hook for eos storage driver

   https://github.com/cs3org/reva/pull/3234

 * Enhancement #3347: Implemented PROPFIND with 0 depth

   https://github.com/cs3org/reva/pull/3347

 * Enhancement #3056: Add public share auth provider

   Add a public share auth middleware

   https://github.com/cs3org/reva/pull/3056

 * Enhancement #3305: Add description to public link

   https://github.com/cs3org/reva/pull/3305

 * Enhancement #3163: Add support for quicklinks for public shares

   https://github.com/cs3org/reva/pull/3163
   https://github.com/cs3org/reva/pull/2715

 * Enhancement #3289: Make Refresh Lock operation WOPI compliant

   We now support the WOPI compliant `UnlockAndRelock` operation. This has been implemented in
   the Eos FS. To make use of it, we need a compatible WOPI server.

   https://github.com/cs3org/reva/pull/3289
   https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/rest/files/unlockandrelock

 * Enhancement #3315: Accept reva token as a bearer authentication

   https://github.com/cs3org/reva/pull/3315

 * Enhancement #3438: Sanitize non-utf8 characters in xattr values in EOS

   https://github.com/cs3org/reva/pull/3438

 * Enhancement #3221: Site Accounts improvements

   The site accounts admin panel has been reworked and now also shows which sites aren't
   configured properly yet. Furthermore, a bug that prevented users from changing site
   configurations has been fixed.

   https://github.com/cs3org/reva/pull/3221

 * Enhancement #3404: Site accounts & Mentix updates

   Some small improvements to the Site Accounts and Mentix services, including normalization of
   data exposed at the `/cs3` endpoint of Mentix.

   https://github.com/cs3org/reva/pull/3404

 * Enhancement #3424: Expire tokens on sunday

   https://github.com/cs3org/reva/pull/3424

 * Enhancement #2986: Use email as display name for external users opening WOPI apps

   We use now the email claim for external/federated accounts as the `username` that is then
   passed to the wopiserver and used as `displayName` in the WOPI context.

   https://github.com/cs3org/reva/pull/2986


