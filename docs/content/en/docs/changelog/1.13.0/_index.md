
---
title: "v1.13.0"
linkTitle: "v1.13.0"
weight: 40
description: >
  Changelog for Reva v1.13.0 (2021-09-14)
---

Changelog for reva 1.13.0 (2021-09-14)
=======================================

The following sections list the changes in reva 1.13.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2024: Fixes for http appprovider endpoints
 * Fix #2054: Fix the response after deleting a share
 * Fix #2026: Fix moving of a shared file
 * Fix #2047: Do not truncate logs on restart
 * Fix #1605: Allow to expose full paths in OCS API
 * Fix #2033: Fix the storage id of shares
 * Fix #2059: Remove "Got registration for user manager" print statements
 * Fix #2051: Remove malformed parameters from WOPI discovery URLs
 * Fix #2055: Fix uploads of empty files
 * Fix #1991: Remove share references when declining shares
 * Fix #2030: Fix superfluous WriteHeader on file upload
 * Enh #2034: Fail initialization of a WOPI AppProvider if
 * Enh #1968: Use a URL object in OpenInAppResponse
 * Enh #1698: Implement folder download as archive
 * Enh #2042: Escape ldap filters
 * Enh #2028: Machine auth provider
 * Enh #2043: Nextcloud user backend
 * Enh #2006: Move ocs API to go-chi/chi based URL routing
 * Enh #1994: Add owncloudsql driver for the userprovider
 * Enh #1971: Add documentation for runtime-plugins
 * Enh #2044: Add utility methods for creating share filters
 * Enh #2065: New sharing role Manager
 * Enh #2015: Add spaces to the list of capabilities
 * Enh #2041: Create operations for Spaces
 * Enh #2029: Tracing agent configuration

Details
-------

 * Bugfix #2024: Fixes for http appprovider endpoints

   https://github.com/cs3org/reva/pull/2024
   https://github.com/cs3org/reva/pull/1968

 * Bugfix #2054: Fix the response after deleting a share

   Added the deleted share to the response after deleting it.

   https://github.com/cs3org/reva/pull/2054

 * Bugfix #2026: Fix moving of a shared file

   As the share receiver, moving a shared file to another share was not possible.

   https://github.com/cs3org/reva/pull/2026

 * Bugfix #2047: Do not truncate logs on restart

   This change fixes the way log files were opened. Before they were truncated and now the log file
   will be open in append mode and created it if it does not exist.

   https://github.com/cs3org/reva/pull/2047

 * Bugfix #1605: Allow to expose full paths in OCS API

   Before this fix a share file_target was always harcoded to use a base path. This fix provides the
   possiblity to expose full paths in the OCIS API and asymptotically in OCIS web.

   https://github.com/cs3org/reva/pull/1605

 * Bugfix #2033: Fix the storage id of shares

   The storageid in the share object contained an incorrect value.

   https://github.com/cs3org/reva/pull/2033

 * Bugfix #2059: Remove "Got registration for user manager" print statements

   Removed the "Got registration for user manager" print statements which spams the log output
   without respecting any log level.

   https://github.com/cs3org/reva/pull/2059

 * Bugfix #2051: Remove malformed parameters from WOPI discovery URLs

   This change fixes the parsing of WOPI discovery URLs for MSOffice /hosting/discovery
   endpoint. This endpoint is known to contain malformed query paramters and therefore this fix
   removes them.

   https://github.com/cs3org/reva/pull/2051

 * Bugfix #2055: Fix uploads of empty files

   This change fixes upload of empty files. Previously this was broken and only worked for the
   owncloud filesystem as it bypasses the semantics of the InitiateFileUpload call to touch a
   local file.

   https://github.com/cs3org/reva/pull/2055

 * Bugfix #1991: Remove share references when declining shares

   Implemented the removal of share references when a share gets declined. Now when a user
   declines a share it will no longer be listed in their `Shares` directory.

   https://github.com/cs3org/reva/pull/1991

 * Bugfix #2030: Fix superfluous WriteHeader on file upload

   Removes superfluous Writeheader on file upload and therefore removes the error message
   "http: superfluous response.WriteHeader call from
   github.com/cs3org/reva/internal/http/interceptors/log.(*responseLogger).WriteHeader
   (log.go:154)"

   https://github.com/cs3org/reva/pull/2030

 * Enhancement #2034: Fail initialization of a WOPI AppProvider if

   The underlying app is not WOPI-compliant nor it is supported by the WOPI bridge extensions

   https://github.com/cs3org/reva/pull/2034

 * Enhancement #1968: Use a URL object in OpenInAppResponse

   https://github.com/cs3org/reva/pull/1968

 * Enhancement #1698: Implement folder download as archive

   Adds a new http service which will create an archive (platform dependent, zip in windows and tar
   in linux) given a list of file.

   https://github.com/cs3org/reva/issues/1698
   https://github.com/cs3org/reva/pull/2066

 * Enhancement #2042: Escape ldap filters

   Added ldap filter escaping to increase the security of reva.

   https://github.com/cs3org/reva/pull/2042

 * Enhancement #2028: Machine auth provider

   Adds a new authentication method used to impersonate users, using a shared secret, called
   api-key.

   https://github.com/cs3org/reva/pull/2028

 * Enhancement #2043: Nextcloud user backend

   Adds Nextcloud as a user backend (Nextcloud drivers for 'auth' and 'user'). Also adds back the
   Nextcloud storage integration tests.

   https://github.com/cs3org/reva/pull/2043

 * Enhancement #2006: Move ocs API to go-chi/chi based URL routing

   https://github.com/cs3org/reva/issues/1986
   https://github.com/cs3org/reva/pull/2006

 * Enhancement #1994: Add owncloudsql driver for the userprovider

   We added a new backend for the userprovider that is backed by an owncloud 10 database. By default
   the `user_id` column is used as the reva user username and reva user opaque id. When setting
   `join_username=true` the reva user username is joined from the `oc_preferences` table
   (`appid='core' AND configkey='username'`) instead. When setting
   `join_ownclouduuid=true` the reva user opaqueid is joined from the `oc_preferences` table
   (`appid='core' AND configkey='ownclouduuid'`) instead. This allows more flexible
   migration strategies. It also supports a `enable_medial_search` config option when
   searching users that will enclose the query with `%`.

   https://github.com/cs3org/reva/pull/1994

 * Enhancement #1971: Add documentation for runtime-plugins

   https://github.com/cs3org/reva/pull/1971

 * Enhancement #2044: Add utility methods for creating share filters

   Updated the CS3 API to include the new share grantee filter and added utility methods for
   creating share filters. This will help making the code more concise.

   https://github.com/cs3org/reva/pull/2044

 * Enhancement #2065: New sharing role Manager

   The new Manager role is equivalent to a Co-Owner with the difference that a Manager can create
   grants on the root of the Space. This means inviting a user to a space will not require an action
   from them, as the Manager assigns the grants.

   https://github.com/cs3org/reva/pull/2065

 * Enhancement #2015: Add spaces to the list of capabilities

   In order for clients to be aware of the new spaces feature we need to enable the `spaces` flag on
   the capabilities' endpoint.

   https://github.com/cs3org/reva/pull/2015

 * Enhancement #2041: Create operations for Spaces

   DecomposedFS is aware now of the concept of Spaces, and supports for creating them.

   https://github.com/cs3org/reva/pull/2041

 * Enhancement #2029: Tracing agent configuration

   Earlier we could only use the collector URL directly, but since an agent can be deployed as a
   sidecar process it makes much more sense to use it instead of the collector directly.

   https://github.com/cs3org/reva/pull/2029


