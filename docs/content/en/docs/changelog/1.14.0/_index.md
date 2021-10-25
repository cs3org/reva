
---
title: "v1.14.0"
linkTitle: "v1.14.0"
weight: 40
description: >
  Changelog for Reva v1.14.0 (2021-10-12)
---

Changelog for reva 1.14.0 (2021-10-12)
=======================================

The following sections list the changes in reva 1.14.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2103: AppProvider: propagate back errors reported by WOPI
 * Fix #2149: Remove excess info from the http list app providers endpoint
 * Fix #2114: Add as default app while registering and skip unset mimetypes
 * Fix #2095: Fix app open when multiple app providers are present
 * Fix #2135: Make TUS capabilities configurable
 * Fix #2076: Fix chi routing
 * Fix #2077: Fix concurrent registration of mimetypes
 * Fix #2154: Return OK when trying to delete a non existing reference
 * Fix #2078: Fix nil pointer exception in stat
 * Fix #2073: Fix opening a readonly filetype with WOPI
 * Fix #2140: Map GRPC error codes to REVA errors
 * Fix #2147: Follow up of #2138: this is the new expected format
 * Fix #2116: Differentiate share types when retrieving received shares in sql driver
 * Fix #2074: Fix Stat() for EOS storage provider
 * Fix #2151: Fix return code for webdav uploads when the token expired
 * Chg #2121: Sharemanager API change
 * Enh #2090: Return space name during list storage spaces
 * Enh #2138: Default AppProvider on top of the providers list
 * Enh #2137: Revamp app registry and add parameter to control file creation
 * Enh #145: UI improvements for the AppProviders
 * Enh #2088: Add archiver and app provider to ocs capabilities
 * Enh #2537: Add maximum files and size to archiver capabilities
 * Enh #2100: Add support for resource id to the archiver
 * Enh #2158: Augment the Id of new spaces
 * Enh #2085: Make encoding user groups in access tokens configurable
 * Enh #146: Filter the denial shares (permission = 0) out of
 * Enh #2141: Use golang v1.17
 * Enh #2053: Safer defaults for TLS verification on LDAP connections
 * Enh #2115: Reduce code duplication in LDAP related drivers
 * Enh #1989: Add redirects from OC10 URL formats
 * Enh #2479: Limit publicshare and resourceinfo scope content
 * Enh #2071: Implement listing favorites via the dav report API
 * Enh #2091: Nextcloud share managers
 * Enh #2070: More unit tests for the Nextcloud storage provider
 * Enh #2087: More unit tests for the Nextcloud auth and user managers
 * Enh #2075: Make owncloudsql leverage existing filecache index
 * Enh #2050: Add a share types filter to the OCS API
 * Enh #2134: Use space Type from request
 * Enh #2132: Align local tests with drone setup
 * Enh #2095: Whitelisting for apps
 * Enh #2155: Pass an extra query parameter to WOPI /openinapp with a

Details
-------

 * Bugfix #2103: AppProvider: propagate back errors reported by WOPI

   On /app/open and return base64-encoded fileids on /app/new

   https://github.com/cs3org/reva/pull/2103

 * Bugfix #2149: Remove excess info from the http list app providers endpoint

   We've removed excess info from the http list app providers endpoint. The app provider section
   contained all mime types supported by a certain app provider, which led to a very big JSON
   payload and since they are not used they have been removed again. Mime types not on the mime type
   configuration list always had `application/octet-stream` as a file extension and
   `APPLICATION/OCTET-STREAM file` as name and description. Now these information are just
   omitted.

   https://github.com/cs3org/reva/pull/2149
   https://github.com/owncloud/ocis/pull/2603
   https://github.com/cs3org/reva/pull/2138

 * Bugfix #2114: Add as default app while registering and skip unset mimetypes

   We fixed that app providers will be set as default app while registering if configured. Also we
   changed the behaviour that listing supported mimetypes only displays allowed / configured
   mimetypes.

   https://github.com/cs3org/reva/pull/2114
   https://github.com/cs3org/reva/pull/2095

 * Bugfix #2095: Fix app open when multiple app providers are present

   We've fixed the gateway behavior, that when multiple app providers are present, it always
   returned that we have duplicate names for app providers. This was due the call to
   GetAllProviders() without any subsequent filtering by name. Now this filter mechanism is in
   place and the duplicate app providers error will only appear if a real duplicate is found.

   https://github.com/cs3org/reva/issues/2095
   https://github.com/cs3org/reva/pull/2117

 * Bugfix #2135: Make TUS capabilities configurable

   We've fixed the configuration for the TUS capabilities, which will now take the given
   configuration instead of always using hardcoded defaults.

   https://github.com/cs3org/reva/pull/2135

 * Bugfix #2076: Fix chi routing

   Chi routes based on the URL.RawPath, which is not updated by the shiftPath based routing used in
   reva. By setting the RawPath to an empty string chi will fall pack to URL.Path, allowing it to
   match percent encoded path segments, e.g. when trying to match emails or multibyte
   characters.

   https://github.com/cs3org/reva/pull/2076

 * Bugfix #2077: Fix concurrent registration of mimetypes

   We fixed registering mimetypes in the mime package when starting multiple storage providers
   in the same process.

   https://github.com/cs3org/reva/pull/2077

 * Bugfix #2154: Return OK when trying to delete a non existing reference

   When the gateway declines a share we can ignore a non existing reference.

   https://github.com/cs3org/reva/pull/2154
   https://github.com/owncloud/ocis/pull/2603

 * Bugfix #2078: Fix nil pointer exception in stat

   https://github.com/cs3org/reva/pull/2078

 * Bugfix #2073: Fix opening a readonly filetype with WOPI

   This change fixes the opening of filetypes that are only supported to be viewed and not to be
   edited by some WOPI compliant office suites.

   https://github.com/cs3org/reva/pull/2073

 * Bugfix #2140: Map GRPC error codes to REVA errors

   We've fixed the error return behaviour in the gateway which would return GRPC error codes from
   the auth middleware. Now it returns REVA errors which other parts of REVA are also able to
   understand.

   https://github.com/cs3org/reva/pull/2140

 * Bugfix #2147: Follow up of #2138: this is the new expected format

   For the mime types configuration for the AppRegistry.

   https://github.com/cs3org/reva/pull/2147

 * Bugfix #2116: Differentiate share types when retrieving received shares in sql driver

   https://github.com/cs3org/reva/pull/2116

 * Bugfix #2074: Fix Stat() for EOS storage provider

   This change fixes the convertion between the eosclient.FileInfo to ResourceInfo, in which
   the field ArbitraryMetadata was missing. Moreover, to be consistent with
   SetArbitraryMetadata() EOS implementation, all the "user." prefix are stripped out from the
   xattrs.

   https://github.com/cs3org/reva/pull/2074

 * Bugfix #2151: Fix return code for webdav uploads when the token expired

   We've fixed the behavior webdav uploads when the token expired before the final stat.
   Previously clients would receive a http 500 error which is wrong, because the file was
   successfully uploaded and only the stat couldn't be performed. Now we return a http 200 ok and
   the clients will fetch the file info in a separate propfind request.

   Also we introduced the upload expires header on the webdav/TUS and datagateway endpoints, to
   signal clients how long an upload can be performed.

   https://github.com/cs3org/reva/pull/2151

 * Change #2121: Sharemanager API change

   This PR updates reva to reflect the share manager CS3 API changes.

   https://github.com/cs3org/reva/pull/2121

 * Enhancement #2090: Return space name during list storage spaces

   In the decomposedfs we return now the space name in the response which is stored in the extended
   attributes.

   https://github.com/cs3org/reva/issues/2090

 * Enhancement #2138: Default AppProvider on top of the providers list

   For each mime type

   Now for each mime type, when asking for the list of mime types, the default AppProvider, set both
   using the config and the SetDefaultProviderForMimeType method, is always in the top of the
   list of AppProviders. The config for the Providers and Mime Types for the AppRegistry changed,
   using a list instead of a map. In fact the list of mime types returned by ListSupportedMimeTypes
   is now ordered according the config.

   https://github.com/cs3org/reva/pull/2138

 * Enhancement #2137: Revamp app registry and add parameter to control file creation

   https://github.com/cs3org/reva/pull/2137

 * Enhancement #145: UI improvements for the AppProviders

   Mime types and their friendly names are now handled in the /app/list HTTP endpoint, and an
   additional /app/new endpoint is made available to create new files for apps.

   https://github.com/cs3org/cs3apis/pull/145
   https://github.com/cs3org/reva/pull/2067

 * Enhancement #2088: Add archiver and app provider to ocs capabilities

   The archiver and app provider has been added to the ocs capabilities.

   https://github.com/cs3org/reva/pull/2088
   https://github.com/owncloud/ocis/pull/2529

 * Enhancement #2537: Add maximum files and size to archiver capabilities

   We added the maximum files count and maximum archive size of the archiver to the capabilities
   endpoint. Clients can use this to generate warnings before the actual archive creation fails.

   https://github.com/owncloud/ocis/issues/2537
   https://github.com/cs3org/reva/pull/2105

 * Enhancement #2100: Add support for resource id to the archiver

   Before the archiver only supported resources provided by a path. Now also the resources ID are
   supported in order to specify the content of the archive to download. The parameters accepted
   by the archiver are two: an optional list of `path` (containing the paths of the resources) and
   an optional list of `id` (containing the resources IDs of the resources).

   https://github.com/cs3org/reva/issues/2097
   https://github.com/cs3org/reva/pull/2100

 * Enhancement #2158: Augment the Id of new spaces

   Newly created spaces were missing the Root reference and the storage id in the space id.

   https://github.com/cs3org/reva/issues/2158

 * Enhancement #2085: Make encoding user groups in access tokens configurable

   https://github.com/cs3org/reva/pull/2085

 * Enhancement #146: Filter the denial shares (permission = 0) out of

   The Shared-with-me UI view. Also they work regardless whether they are accepted or not,
   therefore there's no point to expose them.

   https://github.com/cs3org/cs3apis/pull/146
   https://github.com/cs3org/reva/pull/2072

 * Enhancement #2141: Use golang v1.17

   https://github.com/cs3org/reva/pull/2141

 * Enhancement #2053: Safer defaults for TLS verification on LDAP connections

   The LDAP client connections were hardcoded to ignore certificate validation errors. Now
   verification is enabled by default and a new config parameter 'insecure' is introduced to
   override that default. It is also possible to add trusted Certificates by using the new
   'cacert' config paramter.

   https://github.com/cs3org/reva/pull/2053

 * Enhancement #2115: Reduce code duplication in LDAP related drivers

   https://github.com/cs3org/reva/pull/2115

 * Enhancement #1989: Add redirects from OC10 URL formats

   Added redirectors for ownCloud 10 URLs. This allows users to continue to use their bookmarks
   from ownCloud 10 in ocis.

   https://github.com/cs3org/reva/pull/1989

 * Enhancement #2479: Limit publicshare and resourceinfo scope content

   We changed the publicshare and resourceinfo scopes to contain only necessary values. This
   reduces the size of the resulting token and also limits the amount of data which can be leaked.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2093

 * Enhancement #2071: Implement listing favorites via the dav report API

   Added filter-files to the dav REPORT API. This enables the listing of favorites.

   https://github.com/cs3org/reva/pull/2071
   https://github.com/cs3org/reva/pull/2086

 * Enhancement #2091: Nextcloud share managers

   Share manager that uses Nextcloud as a backend

   https://github.com/cs3org/reva/pull/2091

 * Enhancement #2070: More unit tests for the Nextcloud storage provider

   Adds more unit tests for the Nextcloud storage provider.

   https://github.com/cs3org/reva/pull/2070

 * Enhancement #2087: More unit tests for the Nextcloud auth and user managers

   Adds more unit tests for the Nextcloud auth manager and the Nextcloud user manager

   https://github.com/cs3org/reva/pull/2087

 * Enhancement #2075: Make owncloudsql leverage existing filecache index

   When listing folders the SQL query now uses an existing index on the filecache table.

   https://github.com/cs3org/reva/pull/2075

 * Enhancement #2050: Add a share types filter to the OCS API

   Added a filter to the OCS API to filter the received shares by type.

   https://github.com/cs3org/reva/pull/2050

 * Enhancement #2134: Use space Type from request

   In the decomposedfs we now use the space type from the request when creating a new space.

   https://github.com/cs3org/reva/issues/2134

 * Enhancement #2132: Align local tests with drone setup

   We fixed running the tests locally and align it with the drone setup.

   https://github.com/cs3org/reva/issues/2132

 * Enhancement #2095: Whitelisting for apps

   AppProvider supported mime types are now overridden in its configuration. A friendly name, a
   description, an extension, an icon and a default app, can be configured in the AppRegistry for
   each mime type.

   https://github.com/cs3org/reva/pull/2095

 * Enhancement #2155: Pass an extra query parameter to WOPI /openinapp with a

   Unique and consistent over time user identifier. The Reva token used so far is not consistent
   (it's per session) and also too long.

   https://github.com/cs3org/reva/pull/2155
   https://github.com/cs3org/wopiserver/pull/48


