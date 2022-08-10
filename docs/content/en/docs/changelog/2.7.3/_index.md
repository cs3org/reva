
---
title: "v2.7.3"
linkTitle: "v2.7.3"
weight: 40
description: >
  Changelog for Reva v2.7.3 (2022-08-09)
---

Changelog for reva 2.7.3 (2022-08-09)
=======================================

The following sections list the changes in reva 2.7.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3109: Fix missing check in MustCheckNodePermissions
 * Fix #3086: Fix crash in ldap authprovider
 * Fix #3094: Allow removing password from public links
 * Fix #3096: Fix user filter
 * Fix #3091: Project spaces need no real owner
 * Fix #3088: Use correct sublogger
 * Enh #3123: Allow stating links that have no permissions
 * Enh #3087: Allow to set LDAP substring filter type
 * Enh #3098: App provider http endpoint uses Form instead of Query
 * Enh #3133: Admins can set quota on all spaces
 * Enh #3117: Update go-ldap to v3.4.4
 * Enh #3095: Upload expiration and cleanup

Details
-------

 * Bugfix #3109: Fix missing check in MustCheckNodePermissions

   We added a missing check to the MustCheckNodePermissions function, so space managers can see
   disabled spaces.

   https://github.com/cs3org/reva/pull/3109

 * Bugfix #3086: Fix crash in ldap authprovider

   We fixed possible crash in the LDAP authprovider caused by a null pointer derefence, when the
   IDP settings of the userprovider are different from the authprovider.

   https://github.com/cs3org/reva/pull/3086

 * Bugfix #3094: Allow removing password from public links

   When using cs3 public link share manager passwords would never be removed. We now remove the
   password when getting an update request with empty password field

   https://github.com/cs3org/reva/pull/3094

 * Bugfix #3096: Fix user filter

   We fixed the user filter to display the users drives properly and allow admins to list other
   users drives.

   https://github.com/cs3org/reva/pull/3096
   https://github.com/cs3org/reva/pull/3110

 * Bugfix #3091: Project spaces need no real owner

   Make it possible to use a non existing user as a space owner.

   https://github.com/cs3org/reva/pull/3091
   https://github.com/cs3org/reva/pull/3136

 * Bugfix #3088: Use correct sublogger

   We no longer log cache updated messages when log level is less verbose than debug.

   https://github.com/cs3org/reva/pull/3088

 * Enhancement #3123: Allow stating links that have no permissions

   We need a way to resolve the id when we have a token. This also needs to work for links that have no
   permissions assigned

   https://github.com/cs3org/reva/pull/3123

 * Enhancement #3087: Allow to set LDAP substring filter type

   We introduced new settings for the user- and groupproviders to allow configuring the LDAP
   filter type for substring search. Possible values are: "initial", "final" and "any" to do
   either prefix, suffix or full substring searches.

   https://github.com/cs3org/reva/pull/3087

 * Enhancement #3098: App provider http endpoint uses Form instead of Query

   We've improved the http endpoint now uses the Form instead of Query to also support
   `application/x-www-form-urlencoded` parameters on the app provider http endpoint.

   https://github.com/cs3org/reva/pull/3098

 * Enhancement #3133: Admins can set quota on all spaces

   Admins which have the correct permissions should be able to set quota on all spaces. This is
   implemented via the existing permissions client.

   https://github.com/cs3org/reva/pull/3133

 * Enhancement #3117: Update go-ldap to v3.4.4

   Updated go-ldap/ldap/v3 to the latest upstream release to include the latest bugfixes.

   https://github.com/cs3org/reva/pull/3117

 * Enhancement #3095: Upload expiration and cleanup

   We made storage providers aware of upload expiration and added an interface for FS which
   support listing and purging expired uploads.

   We also implemented said interface for decomposedfs.

   https://github.com/cs3org/reva/pull/3095
   https://github.com/owncloud/ocis/pull/4256


