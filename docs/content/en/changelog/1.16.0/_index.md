
---
title: "v1.16.0"
linkTitle: "v1.16.0"
weight: 40
description: >
  Changelog for Reva v1.16.0 (2021-11-19)
---

Changelog for reva 1.16.0 (2021-11-19)
=======================================

The following sections list the changes in reva 1.16.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2245: Don't announce search-files capability
 * Fix #2247: Merge user ACLs from EOS to sys ACLs
 * Fix #2279: Return the inode of the version folder for files when listing in EOS
 * Fix #2294: Fix HTTP return code when path is invalid
 * Fix #2231: Fix share permission on a single file in sql share driver (cbox pkg)
 * Fix #2230: Fix open by default app and expose default app
 * Fix #2265: Fix nil pointer exception when resolving members of a group (rest driver)
 * Fix #1214: Fix restoring versions
 * Fix #2254: Fix spaces propfind
 * Fix #2260: Fix unset quota xattr on darwin
 * Fix #5776: Enforce permissions in public share apps
 * Fix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing
 * Fix #2287: Add public link access via mount-ID:token/relative-path to the scope
 * Fix #2244: Fix the permissions response for shared files in the cbox sql driver
 * Enh #2219: Add virtual view tests
 * Enh #2230: Add priority to app providers
 * Enh #2258: Improved error messages from the AppProviders
 * Enh #2119: Add authprovider owncloudsql
 * Enh #2211: Enhance the cbox share sql driver to store accepted group shares
 * Enh #2212: Filter root path according to the agent that makes the request
 * Enh #2237: Skip get user call in eosfs in case previous ones also failed
 * Enh #2266: Callback for the EOS UID cache to retry fetch for failed keys
 * Enh #2215: Aggregrate resource info properties for virtual views
 * Enh #2271: Revamp the favorite manager and add the cbox sql driver
 * Enh #2248: Cache whether a user home was created or not
 * Enh #2282: Return a proper NOT_FOUND error when a user or group is not found
 * Enh #2268: Add the reverseproxy http service
 * Enh #2207: Enable users to list all spaces
 * Enh #2286: Add trace ID to middleware loggers
 * Enh #2251: Mentix service inference
 * Enh #2218: Allow filtering of mime types supported by app providers
 * Enh #2213: Add public link share type to propfind response
 * Enh #2253: Support the file editor role for public links
 * Enh #2208: Reduce redundant stat calls when statting by resource ID
 * Enh #2235: Specify a list of allowed folders/files to be archived
 * Enh #2267: Restrict the paths where share creation is allowed
 * Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
 * Enh #2239: Update toml configs

Details
-------

 * Bugfix #2245: Don't announce search-files capability

   The `dav.reports` capability contained a `search-files` report which is currently not
   implemented. We removed it from the defaults.

   https://github.com/cs3org/reva/pull/2245

 * Bugfix #2247: Merge user ACLs from EOS to sys ACLs

   https://github.com/cs3org/reva/pull/2247

 * Bugfix #2279: Return the inode of the version folder for files when listing in EOS

   https://github.com/cs3org/reva/pull/2279

 * Bugfix #2294: Fix HTTP return code when path is invalid

   Before when a path was invalid, the archiver returned a 500 error code. Now this is fixed and
   returns a 404 code.

   https://github.com/cs3org/reva/pull/2294

 * Bugfix #2231: Fix share permission on a single file in sql share driver (cbox pkg)

   https://github.com/cs3org/reva/pull/2231

 * Bugfix #2230: Fix open by default app and expose default app

   We've fixed the open by default app name behaviour which previously only worked, if the default
   app was configured by the provider address. We also now expose the default app on the
   `/app/list` endpoint to clients.

   https://github.com/cs3org/reva/issues/2230
   https://github.com/cs3org/cs3apis/pull/157

 * Bugfix #2265: Fix nil pointer exception when resolving members of a group (rest driver)

   https://github.com/cs3org/reva/pull/2265

 * Bugfix #1214: Fix restoring versions

   Restoring a version would not remove that version from the version list. Now the behavior is
   compatible to ownCloud 10.

   https://github.com/owncloud/ocis/issues/1214
   https://github.com/cs3org/reva/pull/2270

 * Bugfix #2254: Fix spaces propfind

   Fixed the deep listing of spaces.

   https://github.com/cs3org/reva/pull/2254

 * Bugfix #2260: Fix unset quota xattr on darwin

   Unset quota attributes were creating errors in the logfile on darwin.

   https://github.com/cs3org/reva/pull/2260

 * Bugfix #5776: Enforce permissions in public share apps

   A receiver of a read-only public share could still edit files via apps like Collabora. These
   changes enforce the share permissions in apps used on publicly shared resources.

   https://github.com/owncloud/web/issues/5776
   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/22142214

 * Bugfix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing

   We've fixed the status code to 409 according to the WebDAV standard for MKCOL requests where an
   ancestor is missing. Previously these requests would fail with an different error code (eg.
   500) because of storage driver limitations (eg. oCIS FS cannot handle recursive creation of
   directories).

   https://github.com/owncloud/ocis/issues/2767
   https://github.com/cs3org/reva/pull/2293

 * Bugfix #2287: Add public link access via mount-ID:token/relative-path to the scope

   https://github.com/cs3org/reva/pull/2287

 * Bugfix #2244: Fix the permissions response for shared files in the cbox sql driver

   https://github.com/cs3org/reva/pull/2244

 * Enhancement #2219: Add virtual view tests

   https://github.com/cs3org/reva/pull/2219

 * Enhancement #2230: Add priority to app providers

   Before the order of the list returned by the method FindProviders of app providers depended
   from the order in which the app provider registered themselves. Now, it is possible to specify a
   priority for each app provider, and even if an app provider re-register itself (for example
   after a restart), the order is kept.

   https://github.com/cs3org/reva/pull/2230
   https://github.com/cs3org/cs3apis/pull/157
   https://github.com/cs3org/reva/pull/2263

 * Enhancement #2258: Improved error messages from the AppProviders

   Some rather cryptic messages are now hidden to users, and some others are made more
   user-friendly. Support for multiple locales is still missing and out of scope for now.

   https://github.com/cs3org/reva/pull/2258

 * Enhancement #2119: Add authprovider owncloudsql

   We added an authprovider that can be configured to authenticate against an owncloud classic
   mysql database. It verifies the password from the oc_users table.

   https://github.com/cs3org/reva/pull/2119

 * Enhancement #2211: Enhance the cbox share sql driver to store accepted group shares

   https://github.com/cs3org/reva/pull/2211

 * Enhancement #2212: Filter root path according to the agent that makes the request

   https://github.com/cs3org/reva/pull/2212

 * Enhancement #2237: Skip get user call in eosfs in case previous ones also failed

   https://github.com/cs3org/reva/pull/2237

 * Enhancement #2266: Callback for the EOS UID cache to retry fetch for failed keys

   https://github.com/cs3org/reva/pull/2266

 * Enhancement #2215: Aggregrate resource info properties for virtual views

   https://github.com/cs3org/reva/pull/2215

 * Enhancement #2271: Revamp the favorite manager and add the cbox sql driver

   https://github.com/cs3org/reva/pull/2271

 * Enhancement #2248: Cache whether a user home was created or not

   Previously, on every call, we used to stat the user home to make sure that it existed. Now we cache
   it for a given amount of time so as to avoid repeated calls.

   https://github.com/cs3org/reva/pull/2248

 * Enhancement #2282: Return a proper NOT_FOUND error when a user or group is not found

   https://github.com/cs3org/reva/pull/2282

 * Enhancement #2268: Add the reverseproxy http service

   This PR adds an HTTP service which does the job of authenticating incoming requests via the reva
   middleware before forwarding them to the respective backends. This is useful for extensions
   which do not have the auth mechanisms.

   https://github.com/cs3org/reva/pull/2268

 * Enhancement #2207: Enable users to list all spaces

   Added a permission check if the user has the `list-all-spaces` permission. This enables users
   to list all spaces, even those which they are not members of.

   https://github.com/cs3org/reva/pull/2207

 * Enhancement #2286: Add trace ID to middleware loggers

   https://github.com/cs3org/reva/pull/2286

 * Enhancement #2251: Mentix service inference

   Previously, 4 different services per site had to be created in the GOCDB. This PR removes this
   redundancy by infering all endpoints from a single service entity, making site
   administration a lot easier.

   https://github.com/cs3org/reva/pull/2251

 * Enhancement #2218: Allow filtering of mime types supported by app providers

   https://github.com/cs3org/reva/pull/2218

 * Enhancement #2213: Add public link share type to propfind response

   Added share type for public links to propfind responses.

   https://github.com/cs3org/reva/pull/2213
   https://github.com/cs3org/reva/pull/2257

 * Enhancement #2253: Support the file editor role for public links

   https://github.com/cs3org/reva/pull/2253

 * Enhancement #2208: Reduce redundant stat calls when statting by resource ID

   https://github.com/cs3org/reva/pull/2208

 * Enhancement #2235: Specify a list of allowed folders/files to be archived

   Adds a configuration to the archiver service in order to specify a list of folders (as regex)
   that can be archived.

   https://github.com/cs3org/reva/pull/2235

 * Enhancement #2267: Restrict the paths where share creation is allowed

   This PR limits share creation to certain specified paths. These can be useful when users have
   access to global spaces and virtual views but these should not be sharable.

   https://github.com/cs3org/reva/pull/2267

 * Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

 * Enhancement #2239: Update toml configs

   We updated the local and drone configurations, cleanad up the example configs and removed the
   reva gen subcommand which was generating outdated config.

   https://github.com/cs3org/reva/pull/2239


