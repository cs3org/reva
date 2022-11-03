Changelog for reva 2.11.0 (2022-11-03)
=======================================

The following sections list the changes in reva 2.11.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3282: Use Displayname in wopi apps
*   Fix #3430: Add missing error check in decomposedfs
*   Fix #3298: Make date only expiry dates valid for the whole day
*   Fix #3394: Avoid AppProvider panic
*   Fix #3267: Reduced default cache sizes for smaller memory footprint
*   Fix #3338: Fix malformed uid string in cache
*   Fix #3255: Properly escape oc:name in propfind response
*   Fix #3324: Correct base URL for download URL and href when listing file public links
*   Fix #3278: Fix public share view mode during app open
*   Fix #3377: Fix possible race conditions
*   Fix #3274: Fix "uploader" role permissions
*   Fix #3241: Fix uploading empty files into shares
*   Fix #3251: Make listing xattrs more robust
*   Fix #3287: Return OCS forbidden error when a share already exists
*   Fix #3218: Improve performance when listing received shares
*   Fix #3251: Lock source on move
*   Fix #3238: Return relative used quota amount as a percent value
*   Fix #3279: Polish OCS error responses
*   Fix #3307: Refresh lock in decomposedFS needs to overwrite
*   Fix #3368: Return 404 when no permission to space
*   Fix #3341: Validate s3ng downloads
*   Fix #3284: Prevent nil pointer when requesting user
*   Fix #3257: Fix wopi access to publicly shared files
*   Chg #3267: Decomposedfs no longer stores the idp
*   Chg #3381: Changed Name of the Shares Jail
*   Enh #3381: Add capability for sharing by role
*   Enh #3320: Add the parentID to the ocs and dav responses
*   Enh #3239: Add privatelink to PROPFIND response
*   Enh #3340: Add SpaceOwner to some event
*   Enh #4564: Add SpaceShared event
*   Enh #3297: Update dependencies
*   Enh #4959: Make max lock cycles configurable
*   Enh #1949: Add support for denying access in OCS layer
*   Enh #3224: Make the jsoncs3 share manager cache ttl configurable
*   Enh #3290: Harden file system accesses
*   Enh #3332: Allow to enable TLS for grpc service
*   Enh #3223: Improve CreateShare grpc error reporting
*   Enh #3376: Improve logging
*   Enh #3250: Allow sharing the gateway caches
*   Enh #3240: We now only encode &, < and > in PROPFIND PCDATA
*   Enh #3334: Secure the nats connectin with TLS
*   Enh #3300: Do not leak existence of resources
*   Enh #3233: Allow to override default broker for go-micro base ocdav service
*   Enh #3258: Allow ocdav to share the registry instance with other services
*   Enh #3225: Render file parent id for ocs shares
*   Enh #3222: Support Prefer: return=minimal in PROPFIND
*   Enh #3395: Reduce lock contention issues
*   Enh #3286: Make Refresh Lock operation WOPI compliant
*   Enh #3229: Request counting middleware
*   Enh #3312: Implemented new share filters
*   Enh #3308: Update the ttlcache library
*   Enh #3291: The wopi app driver supports more options

Details
-------

*   Bugfix #3282: Use Displayname in wopi apps

   We now use the users display name in wopi apps.

   https://github.com/cs3org/reva/pull/3282

*   Bugfix #3430: Add missing error check in decomposedfs

   During space creation the decomposedfs now checks for errors when trying to read the root node.
   This prevents a panic by no longer calling InternalPath on the node.

   https://github.com/owncloud/ocis/issues/4961
   https://github.com/cs3org/reva/pull/3430

*   Bugfix #3298: Make date only expiry dates valid for the whole day

   When an expiry date like `2022-09-30` is parsed, we now make it valid for the whole day,
   effectively becoming `2022-09-30 23:59:59`

   https://github.com/cs3org/reva/pull/3298

*   Bugfix #3394: Avoid AppProvider panic

   https://github.com/cs3org/reva/pull/3394
   avoid
   panic
   in
   app
   provider

*   Bugfix #3267: Reduced default cache sizes for smaller memory footprint

   We reduced the default cachesizes of the auth interceptors and the share cache. The default of 1
   Million cache entries was way too high and caused a high memory usage upon startup. Config
   options to set custom cache size where added.

   https://github.com/owncloud/ocis/issues/3267
   https://github.com/owncloud/ocis/issues/4628

*   Bugfix #3338: Fix malformed uid string in cache

   The rediscache returns a uid in the format of `<tablename>uid:<someuid>` in the getter this
   results in issues when trying to delete the key from the cache store, because the Delete
   function will prepend the table name to the string which will not be resolvable in redis (e.g.
   `<tablename><tablename>uid:<somuid>`)

   https://github.com/owncloud/ocis/issues/4772
   https://github.com/cs3org/reva/pull/3338

*   Bugfix #3255: Properly escape oc:name in propfind response

   The oc:name property in the ocdav propfind response might contain XML special characters. We
   now apply the proper escaping on that property.

   https://github.com/owncloud/ocis/issues/4474
   https://github.com/cs3org/reva/pull/3255

*   Bugfix #3324: Correct base URL for download URL and href when listing file public links

   We now build the correct base URL when listing file public links.

   https://github.com/owncloud/ocis/issues/4758
   https://github.com/cs3org/reva/pull/3324

*   Bugfix #3278: Fix public share view mode during app open

   We now set the correct view mode during an app open action when the user is accessing a public
   share.

   https://github.com/cs3org/reva/pull/3278

*   Bugfix #3377: Fix possible race conditions

   We fixed two potential race condition when initializing the shared config structure and when
   setting up caches for the http authentication interceptors.

   https://github.com/cs3org/reva/pull/3377

*   Bugfix #3274: Fix "uploader" role permissions

   We fixed a permission problem on "public upload shares", which allowed to view the content of
   the shared upload folder.

   https://github.com/owncloud/ocis/issues/4657
   https://github.com/cs3org/reva/pull/3274

*   Bugfix #3241: Fix uploading empty files into shares

   We fixed a problem which prevented empty files from being uploaded into shares.

   https://github.com/owncloud/ocis/issues/4383
   https://github.com/cs3org/reva/pull/3241

*   Bugfix #3251: Make listing xattrs more robust

   We fixed a potential race condition when listing xattrs of nodes in concurrency situations

   https://github.com/cs3org/reva/pull/3251

*   Bugfix #3287: Return OCS forbidden error when a share already exists

   We now return OCS 104 / HTTP 403 errors when a user tries to reshare a file with a recipient that
   already has access to a resource.

   https://github.com/owncloud/ocis/issues/4630
   https://github.com/cs3org/reva/pull/3287

*   Bugfix #3218: Improve performance when listing received shares

   We improved the performance when listing received shares by getting rid of superfluous
   GetPath calls and sending stat request directly to the storage provider instead of the
   SharesStorageProvider.

   https://github.com/cs3org/reva/pull/3218

*   Bugfix #3251: Lock source on move

   When moving files until now only the lock of the targeted node would be checked. This could lead
   to strange behaviour when using web editors like only office. With checking the source nodes
   lock too, it is now forbidden to rename a file while it is locked

   https://github.com/cs3org/reva/pull/3251

*   Bugfix #3238: Return relative used quota amount as a percent value

   The ocs/ocs/v1.php/cloud/users/ endpoint was fixed to return the relative amount of used
   quota as a percentage value.

   https://github.com/owncloud/ocis/issues/4357
   https://github.com/cs3org/reva/pull/3238

*   Bugfix #3279: Polish OCS error responses

   We aligned more OCS error responses with oc10

   https://github.com/owncloud/ocis/issues/1799
   https://github.com/cs3org/reva/pull/3279

*   Bugfix #3307: Refresh lock in decomposedFS needs to overwrite

   We fixed a bug in the refresh lock operation in the DecomposedFS. The new lock was appended but
   needs to overwrite the existing one.

   https://github.com/cs3org/reva/pull/3307

*   Bugfix #3368: Return 404 when no permission to space

   WebDAV expects a 409 response when trying to upload into a non existing folder. We fixed the
   implementation to return 404 when a user has no access to a space and still return a 409 when a
   parent folder does not exist (and he has access to the space).

   https://github.com/owncloud/ocis/issues/3561
   https://github.com/cs3org/reva/pull/3368
   https://github.com/cs3org/reva/pull/3300

*   Bugfix #3341: Validate s3ng downloads

   The s3ng download func now returns an error in cases where the requested node blob is unknown or
   the blob size does not match the node meta blob size.

   https://github.com/cs3org/reva/pull/3341

*   Bugfix #3284: Prevent nil pointer when requesting user

   We added additional nil pointer checks in the user and groups providers.

   https://github.com/owncloud/ocis/issues/4703
   https://github.com/cs3org/reva/pull/3284

*   Bugfix #3257: Fix wopi access to publicly shared files

   Wopi requests to single file public shares weren't properly authenticated. I added a new check
   to allow wopi to access files which were publicly shared.

   https://github.com/owncloud/ocis/issues/4382
   https://github.com/cs3org/reva/pull/3257

*   Change #3267: Decomposedfs no longer stores the idp

   We no longer persist the IDP of a user id in decomposedfs grants. As a consequence listing or
   reading Grants no longer returns the IDP for the Creator. It never did for the Grantee. Whatever
   credentials are used to authenticate a user we internally have to create a UUID anyway. Either
   by lookung it up in an external service (eg. LDAP or SIEM) or we autoprovision it.

   https://github.com/cs3org/reva/pull/3267

*   Change #3381: Changed Name of the Shares Jail

   We changed the space name of the shares jail to `Shares`.

   https://github.com/cs3org/reva/pull/3381

*   Enhancement #3381: Add capability for sharing by role

   We added the capability to indicate that the ocs share api supports sharing by role.

   https://github.com/cs3org/reva/pull/3381

*   Enhancement #3320: Add the parentID to the ocs and dav responses

   We added the parent resourceID to the OCS and WebDav responses to enable navigation by ID in the
   web client.

   https://github.com/cs3org/reva/pull/3320

*   Enhancement #3239: Add privatelink to PROPFIND response

   We made it possible to request a privatelink WebDAV property.

   https://github.com/cs3org/reva/pull/3239
   https://github.com/cs3org/reva/pull/3240

*   Enhancement #3340: Add SpaceOwner to some event

   We added a SpaceOwner field to some of the events which can be used by consumers to gain access to
   the affected space.

   https://github.com/cs3org/reva/pull/3340
   https://github.com/cs3org/reva/pull/3350

*   Enhancement #4564: Add SpaceShared event

   We added an event that is emmitted when somebody shares a space.

   https://github.com/owncloud/ocis/issues/4303
   https://github.com/owncloud/ocis/pull/4564
   https://github.com/cs3org/reva/pull/3252

*   Enhancement #3297: Update dependencies

   github.com/mileusna/useragent v1.2.0

   https://github.com/cs3org/reva/pull/3297

*   Enhancement #4959: Make max lock cycles configurable

   When a file is locked the flock library will retry a given amount of times (with a increasing
   sleep time inbetween each round) Until now the max amount of such rounds was hardcoded to `10`.
   Now it is configurable, falling back to a default of `25`

   https://github.com/owncloud/ocis/pull/4959

*   Enhancement #1949: Add support for denying access in OCS layer

   http://github.com/cs3org/reva/pull/1949

*   Enhancement #3224: Make the jsoncs3 share manager cache ttl configurable

   We added a new setting to the jsoncs3 share manager which allows to set the cache ttl.

   https://github.com/cs3org/reva/pull/3224

*   Enhancement #3290: Harden file system accesses

   We have reviewed and hardened file system accesses to prevent any vulnerabilities like
   directory traversal.

   https://github.com/cs3org/reva/pull/3290

*   Enhancement #3332: Allow to enable TLS for grpc service

   We added new configuration settings for the grpc based services allowing to enable transport
   security for the services. By setting:

   ```toml [grpc.tls_settings] enabled = true certificate = "<path/to/cert.pem>" key =
   "<path/to/key.pem>" ```

   TLS transportsecurity is enabled using the supplied certificate. When `enabled` is set to
   `true`, but no certificate and key files are supplied reva will generate temporary
   self-signed certificates at startup (this requires to also configure the clients to disable
   certificate verification, see below).

   The client side can be configured via the shared section. Set this to configure the CA for
   verifying server certificates:

   ```toml [shared.grpc_client_options] tls_mode = "on" tls_cacert =
   "</path/to/cafile.pem>" ```

   To disable server certificate verification (e.g. when using the autogenerated self-signed
   certificates) set:

   ```toml [shared.grpc_client_options] tls_mode = "insecure" ```

   To switch off TLS for the clients (which is also the default):

   ```toml [shared.grpc_client_options] tls_mode = "off" ```

   https://github.com/cs3org/reva/pull/3332

*   Enhancement #3223: Improve CreateShare grpc error reporting

   The errorcode returned by the share provider when creating a share where the sharee is already
   the owner of the shared target is a bit more explicit now. Also debug logging was added for this.

   https://github.com/cs3org/reva/pull/3223

*   Enhancement #3376: Improve logging

   We improved the logging by adding the request id to ocdav, ocs and several other http services.

   https://github.com/cs3org/reva/pull/3376

*   Enhancement #3250: Allow sharing the gateway caches

   We replaced the in memory implementation of the gateway with go-micro stores. The gateways
   `cache_store` defaults to `noop` and can be set to `memory`, `redis` or `etcd`. When setting it
   also set any dataproviders `datatxs.*.cache_store` new config option to the same values so
   they can invalidate the cache when a file has been uploadad.

   Cache instances will be shared between handlers when they use the same configuration in the
   same process to allow the dataprovider to access the same cache as the gateway.

   The `nats-js` implementation requires a limited set of characters in the key and is currently
   known to be broken.

   The `etag_cache_ttl` was removed as it was not used anyway.

   https://github.com/cs3org/reva/pull/3250

*   Enhancement #3240: We now only encode &, < and > in PROPFIND PCDATA

   https://github.com/cs3org/reva/pull/3240

*   Enhancement #3334: Secure the nats connectin with TLS

   Encyrpted the connection to the event broker using TLS. Per default TLS is not used.

   https://github.com/cs3org/reva/pull/3334
   https://github.com/cs3org/reva/pull/3382

*   Enhancement #3300: Do not leak existence of resources

   We are now returning a not found error for more requests to not leak existence of spaces for users
   that do not have access to resources.

   https://github.com/cs3org/reva/pull/3300

*   Enhancement #3233: Allow to override default broker for go-micro base ocdav service

   An option for setting an alternative go-micro Broker was introduced. This can be used to avoid
   ocdav from spawing the (unneeded) default http Broker.

   https://github.com/cs3org/reva/pull/3233

*   Enhancement #3258: Allow ocdav to share the registry instance with other services

   This allows to use the in-memory registry when running all services in a single process.

   https://github.com/owncloud/ocis/issues/3134
   https://github.com/cs3org/reva/pull/3258

*   Enhancement #3225: Render file parent id for ocs shares

   We brought back the `file_parent` property for ocs shares. The spaces concept makes
   navigating by path suboptimal. Having a parent id allows navigating without having to look up
   the full path.

   https://github.com/cs3org/reva/pull/3225

*   Enhancement #3222: Support Prefer: return=minimal in PROPFIND

   To reduce HTTP body size when listing folders we implemented
   https://datatracker.ietf.org/doc/html/rfc8144#section-2.1 to omit the 404 propstat
   part when a `Prefer: return=minimal` header is present.

   https://github.com/cs3org/reva/pull/3222

*   Enhancement #3395: Reduce lock contention issues

   We reduced lock contention during high load by optimistically non-locking when listing the
   extended attributes of a file. Only in case of issues the list is read again while holding a lock.

   https://github.com/cs3org/reva/pull/3395

*   Enhancement #3286: Make Refresh Lock operation WOPI compliant

   We now support the WOPI compliant `UnlockAndRelock` operation. This has been implemented in
   the DecomposedFS. To make use of it, we need a compatible WOPI server.

   https://github.com/cs3org/reva/pull/3286
   https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/rest/files/unlockandrelock

*   Enhancement #3229: Request counting middleware

   We added a request counting `prometheus` HTTP middleware and GRPC interceptor that can be
   configured with a `namespace` and `subsystem` to count the number of requests.

   https://github.com/cs3org/reva/pull/3229

*   Enhancement #3312: Implemented new share filters

   Added share filters for space ID and share state.

   https://github.com/owncloud/ocis/issues/3843
   https://github.com/cs3org/reva/pull/3312

*   Enhancement #3308: Update the ttlcache library

   Updated the ttlcache library version and module path.

   https://github.com/cs3org/reva/pull/3308

*   Enhancement #3291: The wopi app driver supports more options

   We now generate a folderurl that is used in the wopi protocol. It provides an endpoint to go back
   from the app to the containing folder in the file list. In addition to that, we now include the
   UI_LLCC parameter in the app-open URL.

   https://github.com/cs3org/reva/pull/3291
   https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/online/discovery#ui_llcc

