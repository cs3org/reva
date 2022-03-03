Changelog for reva 2.0.0 (2022-03-03)
=======================================

The following sections list the changes in reva 2.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2457: Do not swallow error
 * Fix #2422: Handle non existing spaces correctly
 * Fix #2327: Enable changelog on edge branch
 * Fix #2370: Fixes for apps in public shares, project spaces for EOS driver
 * Fix #2464: Pass spacegrants when adding member to space
 * Fix #2430: Fix aggregated child folder id
 * Fix #2348: Make archiver handle spaces protocol
 * Fix #2452: Fix create space error message
 * Fix #2445: Don't handle ids containing "/" in decomposedfs
 * Fix #2285: Accept new userid idp format
 * Fix #2503: Remove the protection from /v?.php/config endpoints
 * Fix #2462: Public shares path needs to be set
 * Fix #2427: Fix registry caching
 * Fix #2298: Remove share refs from trashbin
 * Fix #2433: Fix shares provider filter
 * Fix #2351: Fix Statcache removing
 * Fix #2374: Fix webdav copy of zero byte files
 * Fix #2336: Handle sending all permissions when creating public links
 * Fix #2440: Add ArbitraryMetadataKeys to statcache key
 * Fix #2582: Keep lock structs in a local map protected by a mutex
 * Fix #2372: Make owncloudsql work with the spaces registry
 * Fix #2416: The registry now returns complete space structs
 * Fix #3066: Fix propfind listing for files
 * Fix #2428: Remove unused home provider from config
 * Fix #2334: Revert fix decomposedfs upload
 * Fix #2415: Services should never return transport level errors
 * Fix #2419: List project spaces for share recipients
 * Fix #2501: Fix spaces stat
 * Fix #2432: Use space reference when listing containers
 * Fix #2572: Wait for nats server on middleware start
 * Fix #2454: Fix webdav paths in PROPFINDS
 * Chg #2329: Activate the statcache
 * Chg #2596: Remove hash from public link urls
 * Chg #2495: Remove the ownCloud storage driver
 * Chg #2527: Store space attributes in decomposedFS
 * Chg #2581: Update hard-coded status values
 * Chg #2524: Use description during space creation
 * Chg #2554: Shard nodes per space in decomposedfs
 * Chg #2576: Harden xattrs errors
 * Chg #2436: Replace template in GroupFilter for UserProvider with a simple string
 * Chg #2429: Make archiver id based
 * Chg #2340: Allow multiple space configurations per provider
 * Chg #2396: The ocdav handler is now spaces aware
 * Chg #2349: Require `ListRecycle` when listing trashbin
 * Chg #2353: Reduce log output
 * Chg #2542: Do not encode webDAV ids to base64
 * Chg #2519: Remove the auto creation of the .space folder
 * Chg #2394: Remove logic from gateway
 * Chg #2023: Add a sharestorageprovider
 * Chg #2234: Add a spaces registry
 * Chg #2339: Fix static registry regressions
 * Chg #2370: Fix static registry regressions
 * Chg #2354: Return not found when updating non existent space
 * Chg #2589: Remove deprecated linter modules
 * Chg #2016: Move wrapping and unwrapping of paths to the storage gateway
 * Enh #2591: Set up App Locks with basic locks
 * Enh #1209: Reva CephFS module v0.2.1
 * Enh #2511: Error handling cleanup in decomposed FS
 * Enh #2516: Cleaned up some code
 * Enh #2512: Consolidate xattr setter and getter
 * Enh #2341: Use CS3 permissions API
 * Enh #2343: Allow multiple space type fileters on decomposedfs
 * Enh #2460: Add locking support to decomposedfs
 * Enh #2540: Refactored the xattrs package in the decomposedfs
 * Enh #2463: Do not log whole nodes
 * Enh #2350: Add file locking methods to the storage and filesystem interfaces
 * Enh #2379: Add new file url of the app provider to the ocs capabilities
 * Enh #2369: Implement TouchFile from the CS3apis
 * Enh #2385: Allow to create new files with the app provider on public links
 * Enh #2397: Product field in OCS version
 * Enh #2393: Update tus/tusd to version 1.8.0
 * Enh #2522: Introduce events
 * Enh #2528: Use an exclcusive write lock when writing multiple attributes
 * Enh #2595: Add integration test for the groupprovider
 * Enh #2439: Ignore handled errors when creating spaces
 * Enh #2500: Invalidate listproviders cache
 * Enh #2345: Don't assume that the LDAP groupid in reva matches the name
 * Enh #2525: Allow using AD UUID as userId values
 * Enh #2584: Allow running userprovider integration tests for the LDAP driver
 * Enh #2585: Add metadata storage layer and indexer
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2278: OIDC driver changes for lightweight users
 * Enh #2315: Add new attributes to public link propfinds
 * Enh #2431: Delete shares when purging spaces
 * Enh #2434: Refactor ocdav into smaller chunks
 * Enh #2524: Add checks when removing space members
 * Enh #2457: Restore spaces that were previously deleted
 * Enh #2498: Include grants in list storage spaces response
 * Enh #2344: Allow listing all storage spaces
 * Enh #2547: Add an if-match check to the storage provider
 * Enh #2486: Update cs3apis to include lock api changes
 * Enh #2526: Upgrade ginkgo to v2

Details
-------

 * Bugfix #2457: Do not swallow error

   Decomposedfs not longer swallows errors when creating a node fails.

   https://github.com/cs3org/reva/pull/2457

 * Bugfix #2422: Handle non existing spaces correctly

   When looking up a space by id we returned the wrong status code.

   https://github.com/cs3org/reva/pull/2422

 * Bugfix #2327: Enable changelog on edge branch

   We added a `branch` flag to the `tools/check-changelog/main.go` to fix changelog checks on
   the edge branch.

   https://github.com/cs3org/reva/pull/2327

 * Bugfix #2370: Fixes for apps in public shares, project spaces for EOS driver

   https://github.com/cs3org/reva/pull/2370

 * Bugfix #2464: Pass spacegrants when adding member to space

   When creating a space grant there should not be created a new space. Unfortunately SpaceGrant
   didn't work when adding members to a space. Now a value is placed in the ctx of the
   storageprovider on which decomposedfs reacts

   https://github.com/cs3org/reva/pull/2464

 * Bugfix #2430: Fix aggregated child folder id

   Propfind now returns the correct id and correctly aggregates the mtime and etag.

   https://github.com/cs3org/reva/pull/2430

 * Bugfix #2348: Make archiver handle spaces protocol

   The archiver can now handle the spaces protocol

   https://github.com/cs3org/reva/pull/2348

 * Bugfix #2452: Fix create space error message

   Create space no longer errors with list spaces error messages.

   https://github.com/cs3org/reva/pull/2452

 * Bugfix #2445: Don't handle ids containing "/" in decomposedfs

   The storageprovider previously checked all ids without checking their validity this lead to
   flaky test because it shouldn't check ids that are used from the public storage provider

   https://github.com/cs3org/reva/pull/2445

 * Bugfix #2285: Accept new userid idp format

   The format for userid idp [changed](https://github.com/cs3org/cs3apis/pull/159) and
   this broke [the ocmd
   tutorial](https://reva.link/docs/tutorials/share-tutorial/#5-1-4-create-the-share)
   This PR makes the provider authorizer interceptor accept both the old and the new string
   format.

   https://github.com/cs3org/reva/issues/2285
   https://github.com/cs3org/reva/issues/2285
   See
   and

 * Bugfix #2503: Remove the protection from /v?.php/config endpoints

   We've removed the protection from the "/v1.php/config" and "/v2.php/config" endpoints to be
   API compatible with ownCloud 10.

   https://github.com/cs3org/reva/issues/2503
   https://github.com/owncloud/ocis/issues/1338

 * Bugfix #2462: Public shares path needs to be set

   We need to set the relative path to the space root for public link shares to identify them in the
   shares list.

   https://github.com/owncloud/ocis/issues/2462
   https://github.com/cs3org/reva/pull/2580

 * Bugfix #2427: Fix registry caching

   We now cache space lookups per user.

   https://github.com/cs3org/reva/pull/2427

 * Bugfix #2298: Remove share refs from trashbin

   https://github.com/cs3org/reva/pull/2298

 * Bugfix #2433: Fix shares provider filter

   The shares storage provider now correctly filters space types

   https://github.com/cs3org/reva/pull/2433

 * Bugfix #2351: Fix Statcache removing

   Removing from statcache didn't work correctly with different setups. Unified and fixed

   https://github.com/cs3org/reva/pull/2351

 * Bugfix #2374: Fix webdav copy of zero byte files

   We've fixed the webdav copy action of zero byte files, which was not performed because the
   webdav api assumed, that zero byte uploads are created when initiating the upload, which was
   recently removed from all storage drivers. Therefore the webdav api also uploads zero byte
   files after initiating the upload.

   https://github.com/cs3org/reva/pull/2374
   https://github.com/cs3org/reva/pull/2309

 * Bugfix #2336: Handle sending all permissions when creating public links

   For backwards compatability we now reduce permissions to readonly when a create public link
   carries all permissions.

   https://github.com/cs3org/reva/issues/2336
   https://github.com/owncloud/ocis/issues/1269

 * Bugfix #2440: Add ArbitraryMetadataKeys to statcache key

   Otherwise stating with and without them would return the same result (because it is cached)

   https://github.com/cs3org/reva/pull/2440

 * Bugfix #2582: Keep lock structs in a local map protected by a mutex

   Make sure that only one go routine or process can get the lock.

   https://github.com/cs3org/reva/pull/2582

 * Bugfix #2372: Make owncloudsql work with the spaces registry

   Owncloudsql now works properly with the spaces registry.

   https://github.com/cs3org/reva/pull/2372

 * Bugfix #2416: The registry now returns complete space structs

   We now return the complete space info, including name, path, owner, etc. instead of only path
   and id.

   https://github.com/cs3org/reva/pull/2416

 * Bugfix #3066: Fix propfind listing for files

   When doing a propfind for a file the result contained the files twice.

   https://github.com/owncloud/ocis/issues/3066
   https://github.com/cs3org/reva/pull/2506

 * Bugfix #2428: Remove unused home provider from config

   The spaces registry does not use a home provider config.

   https://github.com/cs3org/reva/pull/2428

 * Bugfix #2334: Revert fix decomposedfs upload

   Reverting https://github.com/cs3org/reva/pull/2330 to fix it properly

   https://github.com/cs3org/reva/pull/2334

 * Bugfix #2415: Services should never return transport level errors

   The CS3 API adopted the grpc error codes from the [google grpc status
   package](https://github.com/googleapis/googleapis/blob/master/google/rpc/status.proto).
   It also separates transport level errors from application level errors on purpose. This
   allows sending CS3 messages over protocols other than GRPC. To keep that seperation, the
   server side must always return `nil`, even though the code generation for go produces function
   signatures for rpcs with an `error` return property. That allows clients to clearly
   distinguish between transport level errors indicated by `err != nil` the error and
   application level errors by checking the status code.

   https://github.com/cs3org/reva/pull/2415

 * Bugfix #2419: List project spaces for share recipients

   The sharing handler now uses the ListProvider call on the registry when sharing by reference.
   Furthermore, the decomposedfs now checks permissions on the root of a space so that a space is
   listed for users that have access to a space.

   https://github.com/cs3org/reva/pull/2419

 * Bugfix #2501: Fix spaces stat

   When stating a space e.g. the Share Jail and that space contains another space, in this case a
   share then the stat would sometimes get the sub space instead of the Share Jail itself.

   https://github.com/cs3org/reva/pull/2501

 * Bugfix #2432: Use space reference when listing containers

   The propfind handler now uses the reference for a space to make lookups relative.

   https://github.com/cs3org/reva/pull/2432

 * Bugfix #2572: Wait for nats server on middleware start

   Use a retry mechanism to connect to the nats server when it is not ready yet

   https://github.com/cs3org/reva/pull/2572

 * Bugfix #2454: Fix webdav paths in PROPFINDS

   The WebDAV Api was handling paths on spaces propfinds in the wrong way. This has been fixed in the
   WebDAV layer.

   https://github.com/cs3org/reva/pull/2454

 * Change #2329: Activate the statcache

   Activates the cache of stat request/responses in the gateway.

   https://github.com/cs3org/reva/pull/2329

 * Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

 * Change #2495: Remove the ownCloud storage driver

   We've removed the ownCloud storage driver because it was no longer maintained after the
   ownCloud SQL storage driver was added.

   If you have been using the ownCloud storage driver, please switch to the ownCloud SQL storage
   driver which brings you more features and is under active maintenance.

   https://github.com/cs3org/reva/pull/2495

 * Change #2527: Store space attributes in decomposedFS

   We need to store more space attributes in the storage. This implements extended space
   attributes in the decomposedFS

   https://github.com/cs3org/reva/pull/2527

 * Change #2581: Update hard-coded status values

   The hard-coded version and product values have been updated to be consistent in all places in
   the code.

   https://github.com/cs3org/reva/pull/2581

 * Change #2524: Use description during space creation

   We can now use a space description during space creation. We also fixed a bug in the spaces roles.
   Co-owners are now maintainers.

   https://github.com/cs3org/reva/pull/2524

 * Change #2554: Shard nodes per space in decomposedfs

   The decomposedfs changas the on disk layout to shard nodes per space.

   https://github.com/cs3org/reva/pull/2554

 * Change #2576: Harden xattrs errors

   Unwrap the error to get the root error.

   https://github.com/cs3org/reva/pull/2576

 * Change #2436: Replace template in GroupFilter for UserProvider with a simple string

   Previously the "groupfilter" configuration for the UserProvider expected a go-template
   value (based of of an `userpb.UserId` as it's input). And it assumed we could run a single LDAP
   query to get all groups a user is member of by specifying the userid. However most LDAP Servers
   store the GroupMembership by either username (e.g. in memberUID Attribute) or by the user's DN
   (e.g. in member/uniqueMember).

   This change removes the userpb.UserId template processing from the groupfilter and replaces
   it with a single string (the username) to cleanup the config a bit. Existing configs need to be
   update to replace the go template references in `groupfilter` (e.g. `{{.}}` or
   `{{.OpaqueId}}`) with `{{query}}`.

   https://github.com/cs3org/reva/pull/2436

 * Change #2429: Make archiver id based

   The archiver now uses ids to walk the tree instead of paths

   https://github.com/cs3org/reva/pull/2429

 * Change #2340: Allow multiple space configurations per provider

   The spaces registry can now use multiple space configurations to allow personal and project
   spaces on the same provider

   https://github.com/cs3org/reva/pull/2340

 * Change #2396: The ocdav handler is now spaces aware

   It will use LookupStorageSpaces and make only relative requests to the gateway. Temp comment

   https://github.com/cs3org/reva/pull/2396

 * Change #2349: Require `ListRecycle` when listing trashbin

   Previously there was no check, so anyone could list anyones trash

   https://github.com/cs3org/reva/pull/2349

 * Change #2353: Reduce log output

   Reduced log output. Some errors or warnings were logged multiple times or even unnecesarily.

   https://github.com/cs3org/reva/pull/2353

 * Change #2542: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2542
   https://github.com/cs3org/reva/pull/2558

 * Change #2519: Remove the auto creation of the .space folder

   We removed the auto creation of the .space folder because we don't develop this feature
   further.

   https://github.com/cs3org/reva/pull/2519

 * Change #2394: Remove logic from gateway

   The gateway will now hold no logic except forwarding the requests to other services.

   https://github.com/cs3org/reva/pull/2394

 * Change #2023: Add a sharestorageprovider

   This PR adds a ShareStorageProvider which enables us to get rid of a lot of special casing in
   other parts of the code. It also fixes several issues regarding shares and group shares.

   https://github.com/cs3org/reva/pull/2023

 * Change #2234: Add a spaces registry

   Spaces registry is supposed to manage spaces. Read
   `pkg/storage/registry/spaces/Readme.md` for full details

   https://github.com/cs3org/reva/pull/2234

 * Change #2339: Fix static registry regressions

   We fixed some smaller issues with using the static registry which were introduced with the
   spaces registry changes.

   https://github.com/cs3org/reva/pull/2339

 * Change #2370: Fix static registry regressions

   We fixed some smaller issues with using the static registry which were introduced with the
   spaces registry changes.

   https://github.com/cs3org/reva/pull/2370

 * Change #2354: Return not found when updating non existent space

   If a spaceid of a space which is updated doesn't exist, handle it as a not found error.

   https://github.com/cs3org/reva/pull/2354

 * Change #2589: Remove deprecated linter modules

   Replaced the deprecated linter modules with the recommended ones.

   https://github.com/cs3org/reva/pull/2589

 * Change #2016: Move wrapping and unwrapping of paths to the storage gateway

   We've moved the wrapping and unwrapping of reference paths to the storage gateway so that the
   storageprovider doesn't have to know its mount path.

   https://github.com/cs3org/reva/pull/2016

 * Enhancement #2591: Set up App Locks with basic locks

   To set up App Locks basic locks are used now

   https://github.com/cs3org/reva/pull/2591

 * Enhancement #1209: Reva CephFS module v0.2.1

   https://github.com/cs3org/reva/pull/1209

 * Enhancement #2511: Error handling cleanup in decomposed FS

   - Avoid inconsensitencies by cleaning up actions in case of err

   https://github.com/cs3org/reva/pull/2511

 * Enhancement #2516: Cleaned up some code

   - Reduced type conversions []byte <-> string - pre-compile chunking regex

   https://github.com/cs3org/reva/pull/2516

 * Enhancement #2512: Consolidate xattr setter and getter

   - Consolidate all metadata Get's and Set's to central functions. - Cleaner code by reduction of
   casts - Easier to hook functionality like indexing

   https://github.com/cs3org/reva/pull/2512

 * Enhancement #2341: Use CS3 permissions API

   Added calls to the CS3 permissions API to the decomposedfs in order to check the user
   permissions.

   https://github.com/cs3org/reva/pull/2341

 * Enhancement #2343: Allow multiple space type fileters on decomposedfs

   The decomposedfs driver now evaluates multiple space type filters when listing storage
   spaces.

   https://github.com/cs3org/reva/pull/2343

 * Enhancement #2460: Add locking support to decomposedfs

   The decomposedfs now implements application level locking

   https://github.com/cs3org/reva/pull/2460

 * Enhancement #2540: Refactored the xattrs package in the decomposedfs

   The xattrs package now uses the xattr.ENOATTR instead of os.ENODATA or os.ENOATTR to check
   attribute existence.

   https://github.com/cs3org/reva/pull/2540
   https://github.com/cs3org/reva/pull/2541

 * Enhancement #2463: Do not log whole nodes

   It turns out that logging whole node objects is very expensive and also spams the logs quite a
   bit. Instead we just log the node ID now.

   https://github.com/cs3org/reva/pull/2463

 * Enhancement #2350: Add file locking methods to the storage and filesystem interfaces

   We've added the file locking methods from the CS3apis to the storage and filesystem
   interfaces. As of now they are dummy implementations and will only return "unimplemented"
   errors.

   https://github.com/cs3org/reva/pull/2350
   https://github.com/cs3org/cs3apis/pull/160

 * Enhancement #2379: Add new file url of the app provider to the ocs capabilities

   We've added the new file capability of the app provider to the ocs capabilities, so that clients
   can discover this url analogous to the app list and file open urls.

   https://github.com/cs3org/reva/pull/2379
   https://github.com/owncloud/ocis/pull/2884
   https://github.com/owncloud/web/pull/5890#issuecomment-993905242

 * Enhancement #2369: Implement TouchFile from the CS3apis

   We've updated the CS3apis and implemented the TouchFile method.

   https://github.com/cs3org/reva/pull/2369
   https://github.com/cs3org/cs3apis/pull/154

 * Enhancement #2385: Allow to create new files with the app provider on public links

   We've added the option to create files with the app provider on public links.

   https://github.com/cs3org/reva/pull/2385

 * Enhancement #2397: Product field in OCS version

   We've added a new field to the OCS Version, which is supposed to announce the product name. The
   web ui as a client will make use of it to make the backend product and version available (e.g. for
   easier bug reports).

   https://github.com/cs3org/reva/pull/2397

 * Enhancement #2393: Update tus/tusd to version 1.8.0

   We've update tus/tusd to version 1.8.0.

   https://github.com/cs3org/reva/issues/2393
   https://github.com/cs3org/reva/pull/2224

 * Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

 * Enhancement #2528: Use an exclcusive write lock when writing multiple attributes

   The xattr package can use an exclusive write lock when writing multiple extended attributes

   https://github.com/cs3org/reva/pull/2528

 * Enhancement #2595: Add integration test for the groupprovider

   Some new integration tests were added to cover the groupprovider.

   https://github.com/cs3org/reva/pull/2595

 * Enhancement #2439: Ignore handled errors when creating spaces

   The CreateStorageSpace no longer logs all error cases with error level logging

   https://github.com/cs3org/reva/pull/2439

 * Enhancement #2500: Invalidate listproviders cache

   We now invalidate the related listproviders cache entries when updating or deleting a storage
   space.

   https://github.com/cs3org/reva/pull/2500

 * Enhancement #2345: Don't assume that the LDAP groupid in reva matches the name

   This allows using attributes like e.g. `entryUUID` or any custom id attribute as the id for
   groups.

   https://github.com/cs3org/reva/pull/2345

 * Enhancement #2525: Allow using AD UUID as userId values

   Active Directory UUID attributes (like e.g. objectGUID) use the LDAP octectString Syntax. In
   order to be able to use them as userids in reva, they need to be converted to their string
   representation.

   https://github.com/cs3org/reva/pull/2525

 * Enhancement #2584: Allow running userprovider integration tests for the LDAP driver

   We extended the integration test suite for the userprovider to allow running it with an LDAP
   server.

   https://github.com/cs3org/reva/pull/2584

 * Enhancement #2585: Add metadata storage layer and indexer

   We ported over and enhanced the metadata storage layer and indexer from ocis-pkg so that it can
   be used by reva services as well.

   https://github.com/cs3org/reva/pull/2585

 * Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

 * Enhancement #2278: OIDC driver changes for lightweight users

   https://github.com/cs3org/reva/pull/2278

 * Enhancement #2315: Add new attributes to public link propfinds

   Added a new property "oc:signature-auth" to public link propfinds. This is a necessary change
   to be able to support archive downloads in password protected public links.

   https://github.com/cs3org/reva/pull/2315

 * Enhancement #2431: Delete shares when purging spaces

   Implemented the second step of the two step spaces delete process. The first step is marking the
   space as deleted, the second step is actually purging the space. During the second step all
   shares, including public shares, in the space will be deleted. When deleting a space the blobs
   are currently not yet deleted since the decomposedfs will receive some changes soon.

   https://github.com/cs3org/reva/pull/2431
   https://github.com/cs3org/reva/pull/2458

 * Enhancement #2434: Refactor ocdav into smaller chunks

   That increases code clarity and enables testing.

   https://github.com/cs3org/reva/pull/2434

 * Enhancement #2524: Add checks when removing space members

   - Removed owners from project spaces - Prevent deletion of last space manager - Viewers and
   editors can always be deleted - Managers can only be deleted when there will be at least one
   remaining

   https://github.com/cs3org/reva/pull/2524

 * Enhancement #2457: Restore spaces that were previously deleted

   After the first step of the two step delete process an admin can decide to restore the space
   instead of deleting it. This will undo the deletion and all files and shares are accessible
   again

   https://github.com/cs3org/reva/pull/2457

 * Enhancement #2498: Include grants in list storage spaces response

   Added the grants to the response of list storage spaces. This allows service clients to show who
   has access to a space.

   https://github.com/cs3org/reva/pull/2498

 * Enhancement #2344: Allow listing all storage spaces

   To implement the drives api we now list all spaces when no filter is given. The Provider info will
   not contain any spaces as the client is responsible for looking up the spaces.

   https://github.com/cs3org/reva/pull/2344

 * Enhancement #2547: Add an if-match check to the storage provider

   Implement a check for the if-match value in InitiateFileUpload to prevent overwrites of newer
   versions.

   https://github.com/cs3org/reva/pull/2547

 * Enhancement #2486: Update cs3apis to include lock api changes

   https://github.com/cs3org/reva/pull/2486

 * Enhancement #2526: Upgrade ginkgo to v2

   https://github.com/cs3org/reva/pull/2526


