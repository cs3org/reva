Changelog for reva 2.13.2 (2023-05-08)
=======================================

The following sections list the changes in reva 2.13.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3845: Fix propagation
*   Fix #3856: Fix response code
*   Fix #3857: Fix trashbin purge

Details
-------

*   Bugfix #3845: Fix propagation

   Fix propagation in concurrency scenarios

   https://github.com/cs3org/reva/pull/3845

*   Bugfix #3856: Fix response code

   The DeleteStorageSpace method response code has been changed

   https://github.com/cs3org/reva/pull/3856

*   Bugfix #3857: Fix trashbin purge

   We have fixed a nil-pointer-exception, when purging files from the trashbin that do not have a
   parent (any more)

   https://github.com/owncloud/ocis/issues/6245
   https://github.com/cs3org/reva/pull/3857

Changelog for reva 2.13.1 (2023-05-03)
=======================================

The following sections list the changes in reva 2.13.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3843: Allow scope check to impersonate space owners

Details
-------

*   Bugfix #3843: Allow scope check to impersonate space owners

   The publicshare scope check now fakes a user to mint an access token when impersonating a user of
   type `SPACE_OWNER` which is used for project spaces. This fixes downloading archives from
   public link shares in project spaces.

   https://github.com/owncloud/ocis/issues/5229
   https://github.com/cs3org/reva/pull/3843

Changelog for reva 2.13.0 (2023-05-02)
=======================================

The following sections list the changes in reva 2.13.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3570: Return 425 on HEAD
*   Fix #3830: Be more robust when logging errors
*   Fix #3815: Bump micro redis store
*   Fix #3596: Cache CreateHome calls
*   Fix #3823: Deny correctlty in decomposedfs
*   Fix #3826: Add by group index to decomposedfs
*   Fix #3618: Drain body on failed put
*   Fix #3420: EOS grpc fixes
*   Fix #3685: Send fileid on copy
*   Fix #3688: Return 425 on GET
*   Fix #3755: Fix app provider language validation
*   Fix #3800: Fix building for freebsd
*   Fix #3700: Fix caching
*   Fix #3535: Fix ceph driver storage fs implementation
*   Fix #3764: Fix missing CORS config in ocdav service
*   Fix #3710: Fix error when try to delete space without permission
*   Fix #3822: Fix deleting spaces
*   Fix #3718: Fix revad-eos docker image which was failing to build
*   Fix #3559: Fix build on freebsd
*   Fix #3696: Fix ldap filters when checking for enabled users
*   Fix #3767: Decode binary UUID when looking up a users group memberships
*   Fix #3741: Fix listing shares to multiple groups
*   Fix #3834: Return correct error during MKCOL
*   Fix #3841: Fix nil pointer and improve logging
*   Fix #3831: Ignore 'null' mtime on tus upload
*   Fix #3758: Fix public links with enforced password
*   Fix #3814: Fix stat cache access
*   Fix #3650: FreeBSD xattr support
*   Fix #3827: Initialize user cache for decomposedfs
*   Fix #3818: Invalidate cache when deleting space
*   Fix #3812: Filemetadata Cache now deletes keys without listing them first
*   Fix #3817: Pipeline cache deletes
*   Fix #3711: Replace ini metadata backend by messagepack backend
*   Fix #3828: Send quota when listing spaces in decomposedfs
*   Fix #3681: Fix etag of "empty" shares jail
*   Fix #3748: Prevent service from panicking
*   Fix #3816: Write Metadata once
*   Chg #3641: Hide file versions for share receivers
*   Chg #3820: Streamline stores
*   Enh #3732: Make method for detecting the metadata backend public
*   Enh #3789: Add capabilities indicating if user attributes are read-only
*   Enh #3792: Add a prometheus gauge to keep track of active uploads and downloads
*   Enh #3637: Add an ID to each events
*   Enh #3704: Add more information to events
*   Enh #3744: Add LDAP user type attribute
*   Enh #3806: Decomposedfs now supports filtering spaces by owner
*   Enh #3730: Antivirus
*   Enh #3531: Async Postprocessing
*   Enh #3571: Async Upload Improvements
*   Enh #3801: Cache node ids
*   Enh #3690: Check set project space quota permission
*   Enh #3686: User disabling functionality
*   Enh #160: Implement the CS3 Lock API in the EOS storage driver
*   Enh #3575: Fix skip group grant index cleanup
*   Enh #3564: Fix tag pkg
*   Enh #3756: Prepare for GDPR export
*   Enh #3612: Group feature changed event added
*   Enh #3729: Improve decomposedfs performance, esp. with network fs/cache
*   Enh #3697: Improve the ini file metadata backend
*   Enh #3819: Allow creating internal links without permission
*   Enh #3740: Limit concurrency in decomposedfs
*   Enh #3569: Always list shares jail when listing spaces
*   Enh #3788: Make resharing configurable
*   Enh #3674: Introduce ini file based metadata backend
*   Enh #3728: Automatically migrate file metadata from xattrs to messagepack
*   Enh #3807: Name Validation
*   Enh #3574: Opaque space group
*   Enh #3598: Pass estream to Storage Providers
*   Enh #3763: Add a capability for personal data export
*   Enh #3577: Prepare for SSE
*   Enh #3731: Add config option to enforce passwords on public links
*   Enh #3693: Enforce the PublicLink.Write permission
*   Enh #3497: Introduce owncloud 10 publiclink manager
*   Enh #3714: Add global max quota option and quota for CreateHome
*   Enh #3759: Set correct share type when listing shares
*   Enh #3594: Add expiration to user and group shares
*   Enh #3580: Share expired event
*   Enh #3620: Allow a new ShareType `SpaceMembershipGroup`
*   Enh #3609: Space Management Permissions
*   Enh #3655: Add expiration date to space memberships
*   Enh #3697: Add support for redis sentinel caches
*   Enh #3552: Suppress tusd logs
*   Enh #3555: Tags
*   Enh #3785: Increase unit test coverage in the ocdav service
*   Enh #3739: Try to rename uploaded files to their final position
*   Enh #3610: Walk and log chi routes

Details
-------

*   Bugfix #3570: Return 425 on HEAD

   For files in postprocessing return 425 also on `HEAD` requests

   https://github.com/cs3org/reva/pull/3570

*   Bugfix #3830: Be more robust when logging errors

   We fixed a problem where logging errors resulted in a panic.

   https://github.com/cs3org/reva/pull/3830

*   Bugfix #3815: Bump micro redis store

   We updated the micro store implementation for redis to use SCAN instead of KEYS

   https://github.com/cs3org/reva/pull/3815
   https://github.com/cs3org/reva/pull/3809

*   Bugfix #3596: Cache CreateHome calls

   We restored the caching of CreateHome calls getting rid of a lot of internal calls.

   https://github.com/cs3org/reva/pull/3596

*   Bugfix #3823: Deny correctlty in decomposedfs

   Decomposedfs had problems denying resources for groups. This is now fixed

   https://github.com/cs3org/reva/pull/3823

*   Bugfix #3826: Add by group index to decomposedfs

   https://github.com/cs3org/reva/pull/3826

*   Bugfix #3618: Drain body on failed put

   When a put request fails the server would not drain the body. This will lead to `connection
   closed` errors on clients when using http 1

   https://github.com/cs3org/reva/pull/3618

*   Bugfix #3420: EOS grpc fixes

   The shares and the applications were not working with the EOS grpc storage driver. This fixes
   both.

   https://github.com/cs3org/reva/pull/3420

*   Bugfix #3685: Send fileid on copy

   When copying a folder oc-fileid header would not be added (unlinke when copying files) this is
   fixed now.

   https://github.com/cs3org/reva/pull/3685

*   Bugfix #3688: Return 425 on GET

   On ocdav GET endpoint the server will now return `425` instead `500` when the file is being
   processed

   https://github.com/cs3org/reva/pull/3688

*   Bugfix #3755: Fix app provider language validation

   This changes the validation to only look at the first part (tag) of the language code and ignore
   the second part (sub-tag).

   https://github.com/cs3org/reva/pull/3755

*   Bugfix #3800: Fix building for freebsd

   We fixed a problem preventing freebsd builds.

   https://github.com/cs3org/reva/pull/3800

*   Bugfix #3700: Fix caching

   Do not cache files that are in processing.

   https://github.com/cs3org/reva/pull/3700

*   Bugfix #3535: Fix ceph driver storage fs implementation

   We adapted the Ceph driver for the edge branch.

   https://github.com/cs3org/reva/pull/3535

*   Bugfix #3764: Fix missing CORS config in ocdav service

   The ocdav service is started with a go micro wrapper. We needed to add the cors config.

   https://github.com/cs3org/reva/pull/3764

*   Bugfix #3710: Fix error when try to delete space without permission

   When a user without the correct permission tries to delete a storage space, return a
   PermissionDenied error instead of an Internal Error.

   https://github.com/cs3org/reva/pull/3710

*   Bugfix #3822: Fix deleting spaces

   We fixed a problem when trying to delete spaces.

   https://github.com/cs3org/reva/pull/3822

*   Bugfix #3718: Fix revad-eos docker image which was failing to build

   https://github.com/cs3org/reva/pull/3718

*   Bugfix #3559: Fix build on freebsd

   Building reva on freebsd was broken due to some deviations in return value types from the
   filesystem.

   https://github.com/cs3org/reva/pull/3559

*   Bugfix #3696: Fix ldap filters when checking for enabled users

   This fixes the ldap filters for checking enabled/disabled users.

   https://github.com/cs3org/reva/pull/3696

*   Bugfix #3767: Decode binary UUID when looking up a users group memberships

   The LDAP backend for the users service didn't correctly decode binary UUIDs when looking up a
   user's group memberships.

   https://github.com/cs3org/reva/pull/3767

*   Bugfix #3741: Fix listing shares to multiple groups

   Users can now see the shares to all groups they are a member of.

   https://github.com/cs3org/reva/pull/3741

*   Bugfix #3834: Return correct error during MKCOL

   We need to return a "PreconditionFailed" error if one of the parent folders during a MKCOL does
   not exist.

   https://github.com/cs3org/reva/pull/3834

*   Bugfix #3841: Fix nil pointer and improve logging

   We fixed a nil pointer error due to a wrong log statement and improved the logging in the json
   public share manager.

   https://github.com/cs3org/reva/pull/3841

*   Bugfix #3831: Ignore 'null' mtime on tus upload

   Decomposedfs now ignores 'null' as an mtime

   https://github.com/cs3org/reva/pull/3831

*   Bugfix #3758: Fix public links with enforced password

   Fix the public link update operation in the case that a password is enforced.

   https://github.com/cs3org/reva/pull/3758

*   Bugfix #3814: Fix stat cache access

   We fixed a problem where wrong data was written to and expected from the stat cache.

   https://github.com/cs3org/reva/pull/3814

*   Bugfix #3650: FreeBSD xattr support

   We now properly handle FreeBSD xattr namespaces by leaving out the `user.` prefix. FreeBSD
   adds that automatically.

   https://github.com/cs3org/reva/pull/3650

*   Bugfix #3827: Initialize user cache for decomposedfs

   https://github.com/cs3org/reva/pull/3827

*   Bugfix #3818: Invalidate cache when deleting space

   Decomposedfs now invalidates the cache when deleting a space.

   https://github.com/cs3org/reva/pull/3818

*   Bugfix #3812: Filemetadata Cache now deletes keys without listing them first

   https://github.com/cs3org/reva/pull/3812

*   Bugfix #3817: Pipeline cache deletes

   The gateway now pipelines deleting keys from the stat and provider cache

   https://github.com/cs3org/reva/pull/3817
   https://github.com/cs3org/reva/pull/3809

*   Bugfix #3711: Replace ini metadata backend by messagepack backend

   We replaced the ini metadata backend by a messagepack backend which is more robust and also uses
   less resources.

   https://github.com/cs3org/reva/pull/3711

*   Bugfix #3828: Send quota when listing spaces in decomposedfs

   We now include free, used and remaining quota when listing spaces

   https://github.com/cs3org/reva/pull/3828

*   Bugfix #3681: Fix etag of "empty" shares jail

   Added the correct etag for an empty shares jail in PROPFIND responses.

   https://github.com/owncloud/ocis/issues/5591
   https://github.com/cs3org/reva/pull/3681

*   Bugfix #3748: Prevent service from panicking

   Changelog is unneccessary

   https://github.com/cs3org/reva/pull/3748

*   Bugfix #3816: Write Metadata once

   Decomposedfs now aggregates metadata when creating directories and spaces into a single
   write.

   https://github.com/cs3org/reva/pull/3816

*   Change #3641: Hide file versions for share receivers

   We needed to change the visibility of file versions and hide them to share receivers. Space
   Editors can still see and restore file versions.

   https://github.com/cs3org/reva/pull/3641

*   Change #3820: Streamline stores

   We refactored and streamlined the different caches and stores.

   https://github.com/cs3org/reva/pull/3820
   https://github.com/cs3org/reva/pull/3777

*   Enhancement #3732: Make method for detecting the metadata backend public

   We made a private method for detecting the decomposedfs metadata backend public

   https://github.com/cs3org/reva/pull/3732

*   Enhancement #3789: Add capabilities indicating if user attributes are read-only

   This adds capabilities that indicates if a user attribute is read-only, and by this lets a
   frontend show this to the user.

   https://github.com/cs3org/reva/pull/3789

*   Enhancement #3792: Add a prometheus gauge to keep track of active uploads and downloads

   This adds a prometheus gauge to keep track of active uploads and downloads.

   https://github.com/cs3org/reva/pull/3792

*   Enhancement #3637: Add an ID to each events

   This way it is possible to uniquely identify events across services

   https://github.com/cs3org/reva/pull/3637

*   Enhancement #3704: Add more information to events

   Adds some missing information to a couple of events

   https://github.com/cs3org/reva/pull/3704

*   Enhancement #3744: Add LDAP user type attribute

   Adding an LDAP attribute so that we can distinguish between member and guest users.

   https://github.com/cs3org/reva/pull/3744

*   Enhancement #3806: Decomposedfs now supports filtering spaces by owner

   Requests using the owner filter now make use of the by-user index

   https://github.com/cs3org/reva/pull/3806

*   Enhancement #3730: Antivirus

   Support antivirus functionality (needs seperate antivirus service)

   https://github.com/cs3org/reva/pull/3730

*   Enhancement #3531: Async Postprocessing

   Provides functionality for async postprocessing. This will allow the system to do the
   postprocessing (virusscan, copying of bytes to their final destination, ...) asynchronous
   to the users request. Major change when active.

   https://github.com/cs3org/reva/pull/3531

*   Enhancement #3571: Async Upload Improvements

   Collection of smaller fixes and quality of life improvements for async postprocessing,
   especially the upload part. Contains unit tests, 0 byte uploads, adjusted endpoint responses
   and more. Adjust mtime when requested from upload. Add etag to upload info.

   Https://github.com/cs3org/reva/pull/3556

   https://github.com/cs3org/reva/pull/3571

*   Enhancement #3801: Cache node ids

   Decomposedfs now keeps an in-memory cache for node ids, sparing a lot of ReadLink calls.

   https://github.com/cs3org/reva/pull/3801

*   Enhancement #3690: Check set project space quota permission

   Instead of checking for `set-space-quota` we now check for `Drive.ReadWriteQuota.Project`
   when changing project space quotas.

   https://github.com/cs3org/reva/pull/3690

*   Enhancement #3686: User disabling functionality

   Check if users are enabled or disabled

   https://github.com/cs3org/reva/pull/3686

*   Enhancement #160: Implement the CS3 Lock API in the EOS storage driver

   https://github.com/cs3org/cs3apis/pull/160
   https://github.com/cs3org/reva/pull/2444

*   Enhancement #3575: Fix skip group grant index cleanup

   Turn off the index cleanup for group grants, it doesn't exist and can therefore be skipped.

   https://github.com/cs3org/reva/pull/3575

*   Enhancement #3564: Fix tag pkg

   `tags` pkg needed an option to build the tags struct from a slice of tags. Here it is

   https://github.com/cs3org/reva/pull/3564

*   Enhancement #3756: Prepare for GDPR export

   Prepares GDPR export (export of all data related to a user)

   https://github.com/cs3org/reva/pull/3756

*   Enhancement #3612: Group feature changed event added

   We added a group feature changed event.

   https://github.com/cs3org/reva/pull/3612

*   Enhancement #3729: Improve decomposedfs performance, esp. with network fs/cache

   We improved the performance of decomposedfs, esp. in scenarios where network storage and
   caches are involed.

   https://github.com/cs3org/reva/pull/3729

*   Enhancement #3697: Improve the ini file metadata backend

   We improved the ini backend for file metadata: - Improve performance - Optionally use a reva
   cache for storing the metadata, which helps tremendously when using distributed file
   systems, for example - Allow for using different metadata backends for different storages

   We also switched the s3ng integration tests to the ini backend so we cover both the xattrs and the
   ini backend at the same time.

   https://github.com/cs3org/reva/pull/3697

*   Enhancement #3819: Allow creating internal links without permission

   Allows creating/updating/deleting internal links without `PublicLink.Write` permission

   https://github.com/cs3org/reva/pull/3819

*   Enhancement #3740: Limit concurrency in decomposedfs

   The number of concurrent goroutines used for listing directories in decomposedfs are now
   limited to a configurable number.

   https://github.com/cs3org/reva/pull/3740

*   Enhancement #3569: Always list shares jail when listing spaces

   Changes spaces listing to always include the shares jail, even when no shares where received.
   If there are no received shares the shares jail will have the etag value `DECAFC00FEE`.

   https://github.com/owncloud/ocis/issues/5190
   https://github.com/cs3org/reva/pull/3569

*   Enhancement #3788: Make resharing configurable

   Resharing was always on previously. This makes resharing configurable via the capability

   https://github.com/cs3org/reva/pull/3788

*   Enhancement #3674: Introduce ini file based metadata backend

   We added a new metadata backend for the decomposed storage driver that uses an additional
   `.ini` file to store file metadata. This allows scaling beyond some filesystem specific xattr
   limitations.

   https://github.com/cs3org/reva/pull/3674
   https://github.com/cs3org/reva/pull/3649

*   Enhancement #3728: Automatically migrate file metadata from xattrs to messagepack

   We added a migration which transparently migrates existig file metadata from xattrs to the new
   messagepack format.

   https://github.com/cs3org/reva/pull/3728

*   Enhancement #3807: Name Validation

   Make name validation in ocdav configurable and add a new validation: max lenght

   https://github.com/cs3org/reva/pull/3807

*   Enhancement #3574: Opaque space group

   Extend the opaque map to contain an identifier to see if it is a user or group grant.

   https://github.com/cs3org/reva/pull/3574

*   Enhancement #3598: Pass estream to Storage Providers

   Similar to the data providers we now pass the stream to the `New` func. This will reduce
   connections from storage providers to nats.

   https://github.com/cs3org/reva/pull/3598

*   Enhancement #3763: Add a capability for personal data export

   Personal data export needs to be hidden behind a capability

   https://github.com/cs3org/reva/pull/3763

*   Enhancement #3577: Prepare for SSE

   Prepare for server sent events with some minor changes

   https://github.com/cs3org/reva/pull/3577

*   Enhancement #3731: Add config option to enforce passwords on public links

   Added a new config option to enforce passwords on public links with "Uploader, Editor,
   Contributor" roles.

   https://github.com/cs3org/reva/pull/3731
   https://github.com/cs3org/reva/pull/3716
   https://github.com/cs3org/reva/pull/3698

*   Enhancement #3693: Enforce the PublicLink.Write permission

   Added checks for the "PublicLink.Write" permission when creating or updating public links.

   https://github.com/cs3org/reva/pull/3693

*   Enhancement #3497: Introduce owncloud 10 publiclink manager

   We can now manage the public links in the oc_share table of an owncloud 10 database.

   https://github.com/cs3org/reva/pull/3497

*   Enhancement #3714: Add global max quota option and quota for CreateHome

   Added a global max quota option to limit how much quota can be assigned to spaces. Added a max
   quota value in the spacescapabilities. Added a quota parameter to CreateHome. This is a
   prerequisite for setting a default quota per usertypes.

   https://github.com/cs3org/reva/pull/3714
   https://github.com/cs3org/reva/pull/3682
   https://github.com/cs3org/reva/pull/3678
   https://github.com/cs3org/reva/pull/3671

*   Enhancement #3759: Set correct share type when listing shares

   This fixes so that we can differentiate between guest/member users for shares.

   https://github.com/cs3org/reva/pull/3759

*   Enhancement #3594: Add expiration to user and group shares

   Added expiration to user and group shares. When shares are accessed after expiration the share
   is automatically removed.

   https://github.com/cs3org/reva/pull/3594

*   Enhancement #3580: Share expired event

   We added a share expired event

   https://github.com/cs3org/reva/pull/3580

*   Enhancement #3620: Allow a new ShareType `SpaceMembershipGroup`

   Added a new sharetype for groups that are members of spaces

   https://github.com/cs3org/reva/pull/3620

*   Enhancement #3609: Space Management Permissions

   Added new permissions to manage spaces: `manage space properties` and `disable spaces`

   https://github.com/cs3org/reva/pull/3609

*   Enhancement #3655: Add expiration date to space memberships

   Added an optional expiration date to space memberships to restrict the access in time.

   https://github.com/cs3org/reva/pull/3655
   https://github.com/cs3org/reva/pull/3628

*   Enhancement #3697: Add support for redis sentinel caches

   We added support for redis sentinel. The sentinel configuration is given in the cache node in
   the following form:

   <host>/<name of master>

   E.g.

   10.10.0.207/mymaster

   https://github.com/owncloud/ocis/issues/5645
   https://github.com/cs3org/reva/pull/3697

*   Enhancement #3552: Suppress tusd logs

   `tusd` package would log specific messages regardless of loglevel. This is now changed

   https://github.com/cs3org/reva/pull/3552

*   Enhancement #3555: Tags

   Base functionality for tagging files

   https://github.com/cs3org/reva/pull/3555

*   Enhancement #3785: Increase unit test coverage in the ocdav service

   https://github.com/cs3org/reva/pull/3785

*   Enhancement #3739: Try to rename uploaded files to their final position

   Before files were always copied which is a performance drop if rename can be done. If not,
   fallback to copy is happening.

   https://github.com/cs3org/reva/pull/3739

*   Enhancement #3610: Walk and log chi routes

   On startup we now log all routes handled by chi

   https://github.com/cs3org/reva/pull/3610

Changelog for reva 2.12.0 (2022-11-25)
=======================================

The following sections list the changes in reva 2.12.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3436: Allow updating to internal link
*   Fix #3473: Decomposedfs fix revision download
*   Fix #3482: Decomposedfs propagate sizediff
*   Fix #3449: Don't leak space information on update drive
*   Fix #3470: Add missing events for managing spaces
*   Fix #3472: Fix an oCDAV error message
*   Fix #3452: Fix access to spaces shared via public link
*   Fix #3440: Set proper names and paths for space roots
*   Fix #3437: Refactor delete error handling
*   Fix #3432: Remove share jail fix
*   Fix #3458: Set the Oc-Fileid header when copying items
*   Enh #3441: Cover ocdav with more unit tests
*   Enh #3493: Configurable filelock duration factor in decomposedfs
*   Enh #3397: Reduce lock contention issues

Details
-------

*   Bugfix #3436: Allow updating to internal link

   We now allow updating any link to an internal link when the user has UpdateGrant permissions

   https://github.com/cs3org/reva/pull/3436

*   Bugfix #3473: Decomposedfs fix revision download

   We rewrote the finish upload code to use a write lock when creating and updating node metadata.
   This prevents some cornercases, allows us to calculate the size diff atomically and fixes
   downloading revisions.

   https://github.com/owncloud/ocis/issues/765
   https://github.com/owncloud/ocis/issues/3868
   https://github.com/cs3org/reva/pull/3473

*   Bugfix #3482: Decomposedfs propagate sizediff

   We now propagate the size diff instead of calculating the treesize. This fixes the slower
   upload speeds in large folders.

   https://github.com/owncloud/ocis/issues/5061
   https://github.com/cs3org/reva/pull/3482

*   Bugfix #3449: Don't leak space information on update drive

   There were some problems with the `UpdateDrive` func in decomposedfs when it is called without
   permission - When calling with empty request it would leak the complete drive info - When
   calling with non-empty request it would leak the drive name

   https://github.com/cs3org/reva/pull/3449
   https://github.com/cs3org/reva/pull/3453

*   Bugfix #3470: Add missing events for managing spaces

   We added more events to cover different aspects of managing spaces

   https://github.com/cs3org/reva/pull/3470

*   Bugfix #3472: Fix an oCDAV error message

   We've fixed an error message in the oCDAV service, that said "error doing GET request to data
   service" even if it did a PATCH request to the data gateway. This error message is now fixed.

   https://github.com/cs3org/reva/pull/3472

*   Bugfix #3452: Fix access to spaces shared via public link

   We fixed a problem where downloading archives from spaces which were shared via public links
   was not possible.

   https://github.com/cs3org/reva/pull/3452

*   Bugfix #3440: Set proper names and paths for space roots

   We fixed a problem where the names and paths were not set correctly for space roots.

   https://github.com/cs3org/reva/pull/3440

*   Bugfix #3437: Refactor delete error handling

   We refactored the ocdav delete handler to return the HTTP status code and an error message to
   simplify error handling.

   https://github.com/cs3org/reva/pull/3437

*   Bugfix #3432: Remove share jail fix

   We have removed the share jail check.

   https://github.com/owncloud/ocis/issues/4945
   https://github.com/cs3org/reva/pull/3432

*   Bugfix #3458: Set the Oc-Fileid header when copying items

   We added the Oc-Fileid header in the COPY response for compatibility reasons.

   https://github.com/owncloud/ocis/issues/5039
   https://github.com/cs3org/reva/pull/3458

*   Enhancement #3441: Cover ocdav with more unit tests

   We added unit tests to cover more ocdav handlers: - delete - mkcol - fixes
   https://github.com/owncloud/ocis/issues/4332

   https://github.com/cs3org/reva/pull/3441
   https://github.com/cs3org/reva/pull/3443
   https://github.com/cs3org/reva/pull/3445
   https://github.com/cs3org/reva/pull/3447
   https://github.com/cs3org/reva/pull/3454
   https://github.com/cs3org/reva/pull/3461

*   Enhancement #3493: Configurable filelock duration factor in decomposedfs

   The lock cycle duration factor in decomposedfs can now be changed by setting
   `lock_cycle_duration_factor`.

   https://github.com/cs3org/reva/pull/3493

*   Enhancement #3397: Reduce lock contention issues

   We reduced lock contention during high load by caching the extended attributes of a file for the
   duration of a request.

   https://github.com/cs3org/reva/pull/3397

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
*   Enh #3252: Add SpaceShared event
*   Enh #3297: Update dependencies
*   Enh #3429: Make max lock cycles configurable
*   Enh #3011: Expose capability to deny access in OCS API
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

*   Enhancement #3252: Add SpaceShared event

   We added an event that is emmitted when somebody shares a space.

   https://github.com/owncloud/ocis/issues/4303
   https://github.com/cs3org/reva/pull/3252
   https://github.com/owncloud/ocis/pull/4564

*   Enhancement #3297: Update dependencies

   * github.com/mileusna/useragent v1.2.0

   https://github.com/cs3org/reva/pull/3297

*   Enhancement #3429: Make max lock cycles configurable

   When a file is locked the flock library will retry a given amount of times (with a increasing
   sleep time inbetween each round) Until now the max amount of such rounds was hardcoded to `10`.
   Now it is configurable, falling back to a default of `25`

   https://github.com/cs3org/reva/pull/3429
   https://github.com/owncloud/ocis/pull/4959

*   Enhancement #3011: Expose capability to deny access in OCS API

   http://github.com/cs3org/reva/pull/3011

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

Changelog for reva 2.10.0 (2022-09-09)
=======================================

The following sections list the changes in reva 2.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3210: Jsoncs3 mtime fix
*   Enh #3213: Allow for dumping the public shares from the cs3 publicshare manager
*   Enh #3199: Add support for cs3 storage backends to the json publicshare manager

Details
-------

*   Bugfix #3210: Jsoncs3 mtime fix

   We now correctly update the mtime to only sync when the file changed on the storage.

   https://github.com/cs3org/reva/pull/3210

*   Enhancement #3213: Allow for dumping the public shares from the cs3 publicshare manager

   We enhanced the cs3 publicshare manager to support dumping its content during a publicshare
   manager migration.

   https://github.com/cs3org/reva/pull/3213

*   Enhancement #3199: Add support for cs3 storage backends to the json publicshare manager

   We enhanced the json publicshare manager to support a cs3 storage backend alongside the file
   and memory backends.

   https://github.com/cs3org/reva/pull/3199

Changelog for reva 2.9.0 (2022-09-08)
=======================================

The following sections list the changes in reva 2.9.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3206: Add spaceid when listing share jail mount points
*   Fix #3194: Adds the rootinfo to storage spaces
*   Fix #3201: Fix shareid on PROPFIND
*   Fix #3176: Forbid duplicate shares
*   Fix #3208: Prevent panic in time conversion
*   Fix #3881: Align ocs status code for permission error on publiclink update
*   Enh #3193: Add shareid to PROPFIND
*   Enh #3180: Add canDeleteAllHomeSpaces permission
*   Enh #3203: Added "delete-all-spaces" permission
*   Enh #4322: OCS get share now also handle received shares
*   Enh #3185: Improve ldap authprovider's error reporting
*   Enh #3179: Improve tokeninfo endpoint
*   Enh #3171: Cs3 to jsoncs3 share manager migration
*   Enh #3204: Make the function flockFile private
*   Enh #3192: Enable space members to update shares

Details
-------

*   Bugfix #3206: Add spaceid when listing share jail mount points

   https://github.com/cs3org/reva/pull/3206

*   Bugfix #3194: Adds the rootinfo to storage spaces

   The sympton of the bug were search results not containing permissions

   https://github.com/cs3org/reva/pull/3194

*   Bugfix #3201: Fix shareid on PROPFIND

   Shareid was still not working properly. We need to parse it from the path

   https://github.com/cs3org/reva/pull/3201

*   Bugfix #3176: Forbid duplicate shares

   When sending a CreateShare request twice two shares would be created, one being not
   accessible. This was blocked by web so the issue wasn't obvious. Now it's forbidden to create
   share for a user who already has a share on that same resource

   https://github.com/cs3org/reva/pull/3176

*   Bugfix #3208: Prevent panic in time conversion

   https://github.com/cs3org/reva/pull/3208

*   Bugfix #3881: Align ocs status code for permission error on publiclink update

   The ocs status code returned for permission errors on updates of publiclink permissions is now
   aligned with the documentation of the OCS share API and the behaviour of ownCloud 10

   https://github.com/owncloud/ocis/issues/3881

*   Enhancement #3193: Add shareid to PROPFIND

   Adds the shareid to the PROPFIND response (in case of shares only)

   https://github.com/cs3org/reva/pull/3193

*   Enhancement #3180: Add canDeleteAllHomeSpaces permission

   We added a permission to the admin role in ocis that allows deleting homespaces on user delete.

   https://github.com/cs3org/reva/pull/3180
   https://github.com/cs3org/reva/pull/3202
   https://github.com/owncloud/ocis/pull/4447/files

*   Enhancement #3203: Added "delete-all-spaces" permission

   We introduced a new permission "delete-all-spaces", users holding this permission are
   allowed to delete any space of any type.

   https://github.com/cs3org/reva/pull/3203

*   Enhancement #4322: OCS get share now also handle received shares

   Requesting a specific share can now also correctly map the path to the mountpoint if the
   requested share is a received share.

   https://github.com/owncloud/ocis/issues/4322
   https://github.com/cs3org/reva/pull/3200

*   Enhancement #3185: Improve ldap authprovider's error reporting

   The errorcode returned by the ldap authprovider driver is a bit more explicit now. (i.e. we
   return a proper Invalid Credentials error now, when the LDAP Bind operation fails with that)

   https://github.com/cs3org/reva/pull/3185

*   Enhancement #3179: Improve tokeninfo endpoint

   We added more information to the tokeninfo endpoint. `aliaslink` is a bool value indicating if
   the permissions are 0. `id` is the full id of the file. Both are available to all users having the
   link token. `spaceType` (indicating the space type) is only available if the user has native
   access

   https://github.com/cs3org/reva/pull/3179

*   Enhancement #3171: Cs3 to jsoncs3 share manager migration

   We added a Load() to the jsoncs3 and Dump() to the sc3 share manager. The shareid might need to be
   prefixed with a storageid and space id.

   https://github.com/cs3org/reva/pull/3171
   https://github.com/cs3org/reva/pull/3195

*   Enhancement #3204: Make the function flockFile private

   Having that function exported is tempting people to use the func to get the name for calling the
   lock functions. That is wrong, as this function is just a helper to generate the lock file name
   from a given file to lock.

   https://github.com/cs3org/reva/pull/3204

*   Enhancement #3192: Enable space members to update shares

   Enabled space members to update shares which they have not created themselves.

   https://github.com/cs3org/reva/pull/3192

Changelog for reva 2.8.0 (2022-08-23)
=======================================

The following sections list the changes in reva 2.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3158: Add name to the propfind response
*   Fix #3157: Fix locking response codes
*   Fix #3152: Disable caching of not found stat responses
*   Fix #4251: Disable caching
*   Chg #3154: Dataproviders now return file metadata
*   Enh #3143: Add /app/open-with-web endpoint
*   Enh #3156: Added language option to the app provider
*   Enh #3148: Add new jsoncs3 share manager

Details
-------

*   Bugfix #3158: Add name to the propfind response

   Previously the file- or foldername had to be extracted from the href. This is not nice and
   doesn't work for alias links.

   https://github.com/cs3org/reva/pull/3158

*   Bugfix #3157: Fix locking response codes

   We've fixed the status codes for locking a file that is already locked.

   https://github.com/owncloud/ocis/issues/4366
   https://github.com/cs3org/reva/pull/3157
   https://github.com/cs3org/reva/pull/3003

*   Bugfix #3152: Disable caching of not found stat responses

   We no longer cache not found responses to prevent concurrent requests interfering with put
   requests.

   https://github.com/owncloud/ocis/issues/4251
   https://github.com/cs3org/reva/pull/3152

*   Bugfix #4251: Disable caching

   We disabled the cache, because there are race condtions that cause tests to fail.

   https://github.com/owncloud/ocis/issues/4251
   https://github.com/cs3org/reva/pull/3167

*   Change #3154: Dataproviders now return file metadata

   Dataprovider drivers can now return file metadata. When the resource info contains a file id,
   the mtime or an etag, these will be included in the response as the corresponding http headers.

   https://github.com/cs3org/reva/pull/3154

*   Enhancement #3143: Add /app/open-with-web endpoint

   We've added an /app/open-with-web endpoint to the app provider, so that clients that are no
   browser or have only limited browser access can also open apps with the help of a Web URL.

   https://github.com/cs3org/reva/pull/3143
   https://github.com/owncloud/ocis/pull/4376

*   Enhancement #3156: Added language option to the app provider

   We've added an language option to the app provider which will in the end be passed to the app a user
   opens so that the web ui is displayed in the users language.

   https://github.com/owncloud/ocis/issues/4367
   https://github.com/cs3org/reva/pull/3156
   https://github.com/owncloud/ocis/pull/4399

*   Enhancement #3148: Add new jsoncs3 share manager

   We've added a new jsoncs3 share manager which splits the json file per storage space and caches
   data locally.

   https://github.com/cs3org/reva/pull/3148

Changelog for reva 2.7.4 (2022-08-10)
=======================================

The following sections list the changes in reva 2.7.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3141: Check ListGrants permission when listing shares

Details
-------

*   Bugfix #3141: Check ListGrants permission when listing shares

   We now check the ListGrants permission when listing outgoing shares. If this permission is
   set, users can list all shares in all spaces.

   https://github.com/cs3org/reva/pull/3141

Changelog for reva 2.7.3 (2022-08-09)
=======================================

The following sections list the changes in reva 2.7.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3109: Fix missing check in MustCheckNodePermissions
*   Fix #3086: Fix crash in ldap authprovider
*   Fix #3094: Allow removing password from public links
*   Fix #3096: Fix user filter
*   Fix #3091: Project spaces need no real owner
*   Fix #3088: Use correct sublogger
*   Enh #3123: Allow stating links that have no permissions
*   Enh #3087: Allow to set LDAP substring filter type
*   Enh #3098: App provider http endpoint uses Form instead of Query
*   Enh #3133: Admins can set quota on all spaces
*   Enh #3117: Update go-ldap to v3.4.4
*   Enh #3095: Upload expiration and cleanup

Details
-------

*   Bugfix #3109: Fix missing check in MustCheckNodePermissions

   We added a missing check to the MustCheckNodePermissions function, so space managers can see
   disabled spaces.

   https://github.com/cs3org/reva/pull/3109

*   Bugfix #3086: Fix crash in ldap authprovider

   We fixed possible crash in the LDAP authprovider caused by a null pointer derefence, when the
   IDP settings of the userprovider are different from the authprovider.

   https://github.com/cs3org/reva/pull/3086

*   Bugfix #3094: Allow removing password from public links

   When using cs3 public link share manager passwords would never be removed. We now remove the
   password when getting an update request with empty password field

   https://github.com/cs3org/reva/pull/3094

*   Bugfix #3096: Fix user filter

   We fixed the user filter to display the users drives properly and allow admins to list other
   users drives.

   https://github.com/cs3org/reva/pull/3096
   https://github.com/cs3org/reva/pull/3110

*   Bugfix #3091: Project spaces need no real owner

   Make it possible to use a non existing user as a space owner.

   https://github.com/cs3org/reva/pull/3091
   https://github.com/cs3org/reva/pull/3136

*   Bugfix #3088: Use correct sublogger

   We no longer log cache updated messages when log level is less verbose than debug.

   https://github.com/cs3org/reva/pull/3088

*   Enhancement #3123: Allow stating links that have no permissions

   We need a way to resolve the id when we have a token. This also needs to work for links that have no
   permissions assigned

   https://github.com/cs3org/reva/pull/3123

*   Enhancement #3087: Allow to set LDAP substring filter type

   We introduced new settings for the user- and groupproviders to allow configuring the LDAP
   filter type for substring search. Possible values are: "initial", "final" and "any" to do
   either prefix, suffix or full substring searches.

   https://github.com/cs3org/reva/pull/3087

*   Enhancement #3098: App provider http endpoint uses Form instead of Query

   We've improved the http endpoint now uses the Form instead of Query to also support
   `application/x-www-form-urlencoded` parameters on the app provider http endpoint.

   https://github.com/cs3org/reva/pull/3098

*   Enhancement #3133: Admins can set quota on all spaces

   Admins which have the correct permissions should be able to set quota on all spaces. This is
   implemented via the existing permissions client.

   https://github.com/cs3org/reva/pull/3133

*   Enhancement #3117: Update go-ldap to v3.4.4

   Updated go-ldap/ldap/v3 to the latest upstream release to include the latest bugfixes.

   https://github.com/cs3org/reva/pull/3117

*   Enhancement #3095: Upload expiration and cleanup

   We made storage providers aware of upload expiration and added an interface for FS which
   support listing and purging expired uploads.

   We also implemented said interface for decomposedfs.

   https://github.com/cs3org/reva/pull/3095
   https://github.com/owncloud/ocis/pull/4256

Changelog for reva 2.7.2 (2022-07-18)
=======================================

The following sections list the changes in reva 2.7.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3079: Allow empty permissions
*   Fix #3084: Spaces relatated permissions and providerid cleanup
*   Fix #3083: Add space id to ItemTrashed event

Details
-------

*   Bugfix #3079: Allow empty permissions

   For alias link we need the ability to set no permission on an link. The permissions will then come
   from the natural permissions the receiving user has on that file/folder

   https://github.com/cs3org/reva/pull/3079

*   Bugfix #3084: Spaces relatated permissions and providerid cleanup

   Following the CS3 resource id refactoring we reverted a logic check when checking the list all
   spaces permission, fixed some typos and made the storageprovider fill in a missing storage
   provider id.

   https://github.com/cs3org/reva/pull/3084

*   Bugfix #3083: Add space id to ItemTrashed event

   We fixed the resource IDs in the ItemTrashed events which were missing the recently introduced
   space ID in the resource ID.

   https://github.com/cs3org/reva/pull/3083

Changelog for reva 2.7.0 (2022-07-15)
=======================================

The following sections list the changes in reva 2.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3075: Check permissions of the move operation destination
*   Fix #3036: Fix revad with EOS docker image
*   Fix #3037: Add uid- and gidNumber to LDAP queries
*   Fix #4061: Forbid resharing with higher permissions
*   Fix #3017: Removed unused gateway config "commit_share_to_storage_ref"
*   Fix #3031: Return proper response code when detecting recursive copy/move operations
*   Fix #3071: Make CS3 sharing drivers parse legacy resource id
*   Fix #3035: Prevent cross space move
*   Fix #3074: Send storage provider and space id to wopi server
*   Fix #3022: Improve the sharing internals
*   Fix #2977: Test valid filename on spaces tus upload
*   Chg #3006: Use spaceID on the cs3api
*   Enh #3043: Introduce LookupCtx for index interface
*   Enh #3009: Prevent recursive copy/move operations
*   Enh #2977: Skip space lookup on space propfind

Details
-------

*   Bugfix #3075: Check permissions of the move operation destination

   We now properly check the permissions on the target of move operations.

   https://github.com/owncloud/ocis/issues/4192
   https://github.com/cs3org/reva/pull/3075

*   Bugfix #3036: Fix revad with EOS docker image

   We've fixed the revad with EOS docker image. Previously the revad binary was build on Alpine and
   not executable on the final RHEL based image.

   https://github.com/cs3org/reva/issues/3036

*   Bugfix #3037: Add uid- and gidNumber to LDAP queries

   For the EOS storage to work correctly the uid- and gidNumber attributes need to be populated.

   https://github.com/cs3org/reva/pull/3037

*   Bugfix #4061: Forbid resharing with higher permissions

   When creating a public link from a viewer share a user was able to set editor permissions on that
   link. This was because of a missing check that is added now

   https://github.com/owncloud/ocis/issues/4061
   https://github.com/owncloud/ocis/issues/3881
   https://github.com/owncloud/ocis/pull/4077

*   Bugfix #3017: Removed unused gateway config "commit_share_to_storage_ref"

   We've removed the unused gateway configuration option "commit_share_to_storage_ref".

   https://github.com/cs3org/reva/pull/3017

*   Bugfix #3031: Return proper response code when detecting recursive copy/move operations

   We changed the ocdav response code to "409 - Conflict" when a recursive operation was detected.

   https://github.com/cs3org/reva/pull/3031

*   Bugfix #3071: Make CS3 sharing drivers parse legacy resource id

   The CS3 public and user sharing drivers will now correct a resource id that is missing a spaceid
   when it can split the storageid.

   https://github.com/cs3org/reva/pull/3071

*   Bugfix #3035: Prevent cross space move

   Decomposedfs now prevents moving across space boundaries

   https://github.com/cs3org/reva/pull/3035

*   Bugfix #3074: Send storage provider and space id to wopi server

   We are now concatenating storage provider id and space id into the endpoint that is sent to the
   wopiserver

   https://github.com/cs3org/reva/issues/3074

*   Bugfix #3022: Improve the sharing internals

   We cleaned up the sharing code validation and comparisons.

   https://github.com/cs3org/reva/pull/3022

*   Bugfix #2977: Test valid filename on spaces tus upload

   Tus uploads in spaces now also test valid filenames.

   https://github.com/owncloud/ocis/issues/3050
   https://github.com/cs3org/reva/pull/2977

*   Change #3006: Use spaceID on the cs3api

   We introduced a new spaceID field on the cs3api to implement the spaces feature in a cleaner way.

   https://github.com/cs3org/reva/pull/3006

*   Enhancement #3043: Introduce LookupCtx for index interface

   The index interface now has a new LookupCtx that can look up multiple values so we can more
   efficiently look up multiple shares by id. It also takes a context so we can pass on the trace
   context to the CS3 backend

   https://github.com/cs3org/reva/pull/3043

*   Enhancement #3009: Prevent recursive copy/move operations

   We changed the ocs API to prevent copying or moving a folder into one of its children.

   https://github.com/cs3org/reva/pull/3009

*   Enhancement #2977: Skip space lookup on space propfind

   We now construct the space id from the /dav/spaces URL intead of making a request to the
   registry.

   https://github.com/owncloud/ocis/issues/1277
   https://github.com/owncloud/ocis/issues/2144
   https://github.com/owncloud/ocis/issues/3073
   https://github.com/cs3org/reva/pull/2977

Changelog for reva 2.7.1 (2022-07-15)
=======================================

The following sections list the changes in reva 2.7.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3080: Make dataproviders return more headers
*   Enh #3046: Add user filter

Details
-------

*   Bugfix #3080: Make dataproviders return more headers

   Instead of ocdav doing an additional Stat request we now rely on the dataprovider to return the
   necessary metadata information as headers.

   https://github.com/owncloud/reva/issues/3080

*   Enhancement #3046: Add user filter

   This PR adds the ability to filter spaces by user-id

   https://github.com/cs3org/reva/pull/3046

Changelog for reva 2.6.1 (2022-06-27)
=======================================

The following sections list the changes in reva 2.6.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2998: Fix 0-byte-uploads
*   Enh #3983: Add capability for alias links
*   Enh #3000: Make less stat requests
*   Enh #3003: Distinguish GRPC FAILED_PRECONDITION and ABORTED codes
*   Enh #3005: Remove unused HomeMapping variable

Details
-------

*   Bugfix #2998: Fix 0-byte-uploads

   We fixed a problem with 0-byte uploads by using TouchFile instead of going through TUS
   (decomposedfs and owncloudsql storage drivers only for now).

   https://github.com/cs3org/reva/pull/2998

*   Enhancement #3983: Add capability for alias links

   For better UX clients need a way to discover if alias links are supported by the server. We added a
   capability under "files_sharing/public/alias"

   https://github.com/owncloud/ocis/issues/3983
   https://github.com/cs3org/reva/pull/2991

*   Enhancement #3000: Make less stat requests

   The /dav/spaces endpoint now constructs a reference instead of making a lookup grpc call,
   reducing the number of requests.

   https://github.com/cs3org/reva/pull/3000

*   Enhancement #3003: Distinguish GRPC FAILED_PRECONDITION and ABORTED codes

   Webdav distinguishes between 412 precondition failed for if match errors for locks or etags,
   uses 405 Method Not Allowed when trying to MKCOL an already existing collection and 409
   Conflict when intermediate collections are missing.

   The CS3 GRPC status codes are modeled after
   https://github.com/googleapis/googleapis/blob/master/google/rpc/code.proto. When
   trying to use the error codes to distinguish these cases on a storageprovider CreateDir call we
   can map ALREADY_EXISTS to 405, FAILED_PRECONDITION to 409 and ABORTED to 412.

   Unfortunately, we currently use and map FAILED_PRECONDITION to 412. I assume, because the
   naming is very similar to PreconditionFailed. However the GRPC docs are very clear that
   ABORTED should be used, specifically mentioning etags and locks.

   With this PR we internally clean up the usage in the decomposedfs and mapping in the ocdav
   handler.

   https://github.com/cs3org/reva/pull/3003
   https://github.com/cs3org/reva/pull/3010

*   Enhancement #3005: Remove unused HomeMapping variable

   We have removed the unused HomeMapping variable from the gateway.

   https://github.com/cs3org/reva/pull/3005

Changelog for reva 2.6.0 (2022-06-21)
=======================================

The following sections list the changes in reva 2.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2985: Make stat requests route based on storage providerid
*   Fix #2987: Let archiver handle all error codes
*   Fix #2994: Fix errors when loading shares
*   Fix #2996: Do not close share dump channels
*   Fix #2993: Remove unused configuration
*   Fix #2950: Fix sharing with space ref
*   Fix #2991: Make sharesstorageprovider get accepted share
*   Chg #2877: Enable resharing
*   Chg #2984: Update CS3Apis
*   Enh #3753: Add executant to the events
*   Enh #2820: Instrument GRPC and HTTP requests with OTel
*   Enh #2975: Leverage shares space storageid and type when listing shares
*   Enh #3882: Explicitly return on ocdav move requests with body
*   Enh #2932: Stat accepted shares mountpoints, configure existing share updates
*   Enh #2944: Improve owncloudsql connection management
*   Enh #2962: Per service TracerProvider
*   Enh #2911: Allow for dumping and loading shares
*   Enh #2938: Sharpen tooling

Details
-------

*   Bugfix #2985: Make stat requests route based on storage providerid

   The gateway now uses a filter mask to only fetch the root id of a space for stat requests. This
   allows the spaces registry to determine the responsible storage provider without querying
   the storageproviders.

   https://github.com/cs3org/reva/pull/2985

*   Bugfix #2987: Let archiver handle all error codes

   We fixed the archiver handler to handle all error codes

   https://github.com/cs3org/reva/pull/2987

*   Bugfix #2994: Fix errors when loading shares

   We fixed a bug where loading shares and associated received shares ran into issues when
   handling them simultaneously.

   https://github.com/cs3org/reva/pull/2994

*   Bugfix #2996: Do not close share dump channels

   We no longer close the channels when dumping shares, it's the responsibility of the caller.

   https://github.com/cs3org/reva/pull/2996

*   Bugfix #2993: Remove unused configuration

   We've fixed removed unused configuration:

   - `insecure` from the dataprovider - `timeout` from the dataprovider - `tmp_folder` from the
   storageprovider

   https://github.com/cs3org/reva/pull/2993

*   Bugfix #2950: Fix sharing with space ref

   We've fixed a bug where share requests with `path` attribute present ignored the `space_ref`
   attribute. We now give the `space_ref` attribute precedence over the `path` attribute.

   https://github.com/cs3org/reva/pull/2950

*   Bugfix #2991: Make sharesstorageprovider get accepted share

   The sharesstorageprovider now gets an accepted share instead of filtering all shares.

   https://github.com/cs3org/reva/pull/2991

*   Change #2877: Enable resharing

   This will allow resharing of files. - All Viewers and Editors are now able to reshare files and
   folders - One can still edit their own shares, even when loosing share permissions - Viewers and
   Editors in a space are not affected

   https://github.com/cs3org/reva/pull/2877

*   Change #2984: Update CS3Apis

   Updated the CS3Apis to make use of field_mask and pagination for list requests.

   https://github.com/cs3org/reva/pull/2984

*   Enhancement #3753: Add executant to the events

   Added the executant field to all events.

   https://github.com/owncloud/ocis/issues/3753
   https://github.com/cs3org/reva/pull/2945

*   Enhancement #2820: Instrument GRPC and HTTP requests with OTel

   We've added the enduser.id tag to the HTTP and GRPC requests. We've fixed the tracer names.
   We've decorated the traces with the hostname.

   https://github.com/cs3org/reva/pull/2820

*   Enhancement #2975: Leverage shares space storageid and type when listing shares

   The list shares call now also fills the storageid to allow the space registry to directly route
   requests to the correct storageprovider. The spaces registry will now also skip
   storageproviders that are not configured for a requested type, causing type 'personal'
   requests to skip the sharestorageprovider.

   https://github.com/cs3org/reva/pull/2975
   https://github.com/cs3org/reva/pull/2980

*   Enhancement #3882: Explicitly return on ocdav move requests with body

   Added a check if a ocdav move request contains a body. If it does a 415 415 (Unsupported Media
   Type) will be returned.

   https://github.com/owncloud/ocis/issues/3882
   https://github.com/cs3org/reva/pull/2974

*   Enhancement #2932: Stat accepted shares mountpoints, configure existing share updates

   https://github.com/cs3org/reva/pull/2932

*   Enhancement #2944: Improve owncloudsql connection management

   The owncloudsql storagedriver is now aware of the request context and will close db
   connections when http connections are closed or time out. We also increased the max number of
   open connections from 10 to 100 to prevent a corner case where all connections were used but idle
   connections were not freed.

   https://github.com/cs3org/reva/pull/2944

*   Enhancement #2962: Per service TracerProvider

   To improve tracing we create separate TracerProviders per service now. This is especially
   helpful when running multiple reva services in a single process (like e.g. oCIS does).

   https://github.com/cs3org/reva/pull/2962
   https://github.com/cs3org/reva/pull/2978

*   Enhancement #2911: Allow for dumping and loading shares

   We now have interfaces for dumpable and loadable share manages which can be used to migrate
   shares between share managers

   https://github.com/cs3org/reva/pull/2911

*   Enhancement #2938: Sharpen tooling

   * We increased the linting timeout to 10min which caused some release builds to time out

   https://github.com/cs3org/reva/pull/2938

Changelog for reva 2.5.1 (2022-06-08)
=======================================

The following sections list the changes in reva 2.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2931: Allow listing share jail space
*   Fix #3704: Fix propfinds with depth 0

Details
-------

*   Bugfix #2931: Allow listing share jail space

   Clients can now list the share jail content via `PROPFIND /dav/spaces/{sharejailid}`

   https://github.com/cs3org/reva/pull/2931

*   Bugfix #3704: Fix propfinds with depth 0

   Fixed the response for propfinds with depth 0. The response now doesn't contain the shares jail
   anymore.

   https://github.com/owncloud/ocis/issues/3704
   https://github.com/cs3org/reva/pull/2918

Changelog for reva 2.5.0 (2022-06-07)
=======================================

The following sections list the changes in reva 2.5.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2909: The decomposedfs now checks the GetPath permission
*   Fix #2899: Empty meta requests should return body
*   Fix #2928: Fix mkcol response code
*   Fix #2907: Correct share jail child aggregation
*   Fix #3810: Fix unlimitted quota in spaces
*   Fix #3498: Check user permissions before updating/removing public shares
*   Fix #2904: Share jail now works properly when accessed as a space
*   Fix #2903: User owncloudsql now uses the correct userid
*   Chg #2920: Clean up the propfind code
*   Chg #2913: Rename ocs parameter "space_ref"
*   Enh #2919: EOS Spaces implementation
*   Enh #2888: Introduce spaces field mask
*   Enh #2922: Refactor webdav error handling

Details
-------

*   Bugfix #2909: The decomposedfs now checks the GetPath permission

   After fixing the meta endpoint and introducing the fieldmask the GetPath call is made directly
   to the storageprovider. The decomposedfs now checks if the current user actually has the
   permission to get the path. Before the two previous PRs this was covered by the list storage
   spaces call which used a stat request and the stat permission.

   https://github.com/cs3org/reva/pull/2909

*   Bugfix #2899: Empty meta requests should return body

   Meta requests with no resourceID should return a multistatus response body with a 404 part.

   https://github.com/cs3org/reva/pull/2899

*   Bugfix #2928: Fix mkcol response code

   We now return the correct response code when an mkcol fails.

   https://github.com/cs3org/reva/pull/2928

*   Bugfix #2907: Correct share jail child aggregation

   We now add up the size of all mount points when aggregating the size for a child with the same name.
   Furthermore, the listing should no longer contain duplicate entries.

   https://github.com/cs3org/reva/pull/2907

*   Bugfix #3810: Fix unlimitted quota in spaces

   Fixed the quota check when unlimitting a space, i.e. when setting the quota to "0".

   https://github.com/owncloud/ocis/issues/3810
   https://github.com/cs3org/reva/pull/2895

*   Bugfix #3498: Check user permissions before updating/removing public shares

   Added permission checks before updating or deleting public shares. These methods previously
   didn't enforce the users permissions.

   https://github.com/owncloud/ocis/issues/3498
   https://github.com/cs3org/reva/pull/3900

*   Bugfix #2904: Share jail now works properly when accessed as a space

   When accessing shares via the virtual share jail we now build correct relative references
   before forwarding the requests to the correct storage provider.

   https://github.com/cs3org/reva/pull/2904

*   Bugfix #2903: User owncloudsql now uses the correct userid

   https://github.com/cs3org/reva/pull/2903

*   Change #2920: Clean up the propfind code

   Cleaned up the ocdav propfind code to make it more readable.

   https://github.com/cs3org/reva/pull/2920

*   Change #2913: Rename ocs parameter "space_ref"

   We decided to deprecate the parameter "space_ref". We decided to use "space" parameter
   instead. The difference is that "space" must not contain a "path". The "path" parameter can be
   used in combination with "space" to create a relative path request

   https://github.com/cs3org/reva/pull/2913

*   Enhancement #2919: EOS Spaces implementation

   https://github.com/cs3org/reva/pull/2919

*   Enhancement #2888: Introduce spaces field mask

   We now use a field mask to select which properties to retrieve when looking up storage spaces.
   This allows the gateway to only ask for `root` when trying to forward id or path based requests.

   https://github.com/cs3org/reva/pull/2888

*   Enhancement #2922: Refactor webdav error handling

   We made more webdav handlers return a status code and error to unify error rendering

   https://github.com/cs3org/reva/pull/2922

Changelog for reva 2.4.1 (2022-05-24)
=======================================

The following sections list the changes in reva 2.4.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2891: Add missing http status code

Details
-------

*   Bugfix #2891: Add missing http status code

   This Fix adds a missing status code to the InsufficientStorage error in reva, to allow tus to
   pass it through.

   https://github.com/cs3org/reva/pull/2891

Changelog for reva 2.4.0 (2022-05-24)
=======================================

The following sections list the changes in reva 2.4.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2854: Handle non uuid space and nodeid in decomposedfs
*   Fix #2853: Filter CS3 share manager listing
*   Fix #2868: Actually remove blobs when purging
*   Fix #2882: Fix FileUploaded event being emitted too early
*   Fix #2848: Fix storage id in the references in the ItemTrashed events
*   Fix #2852: Fix rcbox dependency on reva 1.18
*   Fix #3505: Fix creating a new file with wopi
*   Fix #2885: Move stat out of usershareprovider
*   Fix #2883: Fix role consideration when updating a share
*   Fix #2864: Fix Grant Space IDs
*   Fix #2870: Update quota calculation
*   Fix #2876: Fix version number in status page
*   Fix #2829: Don't include versions in quota
*   Chg #2856: Do not allow to edit disabled spaces
*   Enh #3741: Add download endpoint to ocdav versions API
*   Enh #2884: Show mounted shares in virtual share jail root
*   Enh #2792: Use storageproviderid for spaces routing

Details
-------

*   Bugfix #2854: Handle non uuid space and nodeid in decomposedfs

   The decomposedfs no longer panics when trying to look up spaces with a non uuid length id.

   https://github.com/cs3org/reva/pull/2854

*   Bugfix #2853: Filter CS3 share manager listing

   The cs3 share manager driver now correctly filters user and group queries

   https://github.com/cs3org/reva/pull/2853

*   Bugfix #2868: Actually remove blobs when purging

   Blobs were not being deleted properly on purge. Now if a folder gets purged all its children will
   be deleted

   https://github.com/cs3org/reva/pull/2868

*   Bugfix #2882: Fix FileUploaded event being emitted too early

   We fixed a problem where the FileUploaded event was emitted before the upload had actually
   finished.

   https://github.com/cs3org/reva/pull/2882

*   Bugfix #2848: Fix storage id in the references in the ItemTrashed events

   https://github.com/cs3org/reva/pull/2848

*   Bugfix #2852: Fix rcbox dependency on reva 1.18

   The cbox package no longer depends on reva 1.18.

   https://github.com/cs3org/reva/pull/2852

*   Bugfix #3505: Fix creating a new file with wopi

   Fixed a bug in the appprovider which prevented creating new files.

   https://github.com/owncloud/ocis/issues/3505
   https://github.com/cs3org/reva/pull/2869

*   Bugfix #2885: Move stat out of usershareprovider

   The sharesstorageprovider now only stats the acceptet shares when necessary.

   https://github.com/cs3org/reva/pull/2885

*   Bugfix #2883: Fix role consideration when updating a share

   Previously when updating a share the endpoint only considered the permissions, now this also
   respects a given role.

   https://github.com/cs3org/reva/pull/2883

*   Bugfix #2864: Fix Grant Space IDs

   The opaqueID for a grant space was incorrectly overwritten with the root space id.

   https://github.com/cs3org/reva/pull/2864

*   Bugfix #2870: Update quota calculation

   We now render the `free` and `definition` quota properties, taking into account the remaining
   bytes reported from the storage space and calculating `relative` only when possible.

   https://github.com/cs3org/reva/pull/2870

*   Bugfix #2876: Fix version number in status page

   We needed to undo the version number changes on the status page to keep compatibility for legacy
   clients. We added a new field `productversion` for the actual version of the product.

   https://github.com/cs3org/reva/pull/2876
   https://github.com/cs3org/reva/pull/2889

*   Bugfix #2829: Don't include versions in quota

   Fixed the quota check to not count the quota of previous versions.

   https://github.com/owncloud/ocis/issues/2829
   https://github.com/cs3org/reva/pull/2863

*   Change #2856: Do not allow to edit disabled spaces

   Previously managers could still upload to disabled spaces. This is now forbidden

   https://github.com/cs3org/reva/pull/2856

*   Enhancement #3741: Add download endpoint to ocdav versions API

   Added missing endpoints to the ocdav versions API. This enables downloads of previous file
   versions.

   https://github.com/owncloud/ocis/issues/3741
   https://github.com/cs3org/reva/pull/2855

*   Enhancement #2884: Show mounted shares in virtual share jail root

   The virtual share jail now shows the mounted shares to allow the desktop client to sync that
   collection.

   https://github.com/owncloud/ocis/issues/3719
   https://github.com/cs3org/reva/pull/2884

*   Enhancement #2792: Use storageproviderid for spaces routing

   We made the spaces registry aware of storageprovider ids and use them to route directly to the
   correct storageprovider

   https://github.com/cs3org/reva/pull/2792

Changelog for reva 2.3.1 (2022-05-08)
=======================================

The following sections list the changes in reva 2.3.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2827: Check permissions when deleting spaces
*   Fix #2830: Correctly render response when accepting merged shares
*   Fix #2831: Fix uploads to owncloudsql storage when no mtime is provided
*   Enh #2833: Make status.php values configurable
*   Enh #2832: Add version option for ocdav go-micro service

Details
-------

*   Bugfix #2827: Check permissions when deleting spaces

   Do not allow viewers and editors to delete a space (you need to be manager) Block deleting a space
   via dav service (should use graph to avoid accidental deletes)

   https://github.com/cs3org/reva/pull/2827

*   Bugfix #2830: Correctly render response when accepting merged shares

   We now only return the data for the accepted share instead of concatenating data for all
   affected shares.

   https://github.com/cs3org/reva/pull/2830

*   Bugfix #2831: Fix uploads to owncloudsql storage when no mtime is provided

   We've fixed uploads to owncloudsql storage when no mtime is provided. We now just use the
   current timestamp. Previously the upload did fail.

   https://github.com/cs3org/reva/pull/2831

*   Enhancement #2833: Make status.php values configurable

   We've added an option to set the status values for `product`, `productname`, `version`,
   `versionstring` and `edition`.

   https://github.com/cs3org/reva/pull/2833

*   Enhancement #2832: Add version option for ocdav go-micro service

   We've added an option to set a version for the ocdav go-micro registry. This enables you to set a
   version queriable by from the go-micro registry.

   https://github.com/cs3org/reva/pull/2832

Changelog for reva 2.3.0 (2022-05-02)
=======================================

The following sections list the changes in reva 2.3.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2693: Support editnew actions from MS Office
*   Fix #2588: Dockerfile.revad-ceph to use the right base image
*   Fix #2499: Removed check DenyGrant in resource permission
*   Fix #2285: Accept new userid idp format
*   Fix #2802: Fix the resource id handling for space shares
*   Fix #2800: Fix spaceid parsing in spaces trashbin API
*   Fix #1846: Fix trash-bin propfind responses
*   Fix #2608: Respect the tracing_service_name config variable
*   Fix #2742: Use exact match in login filter
*   Fix #2759: Made uid, gid claims parsing more robust in OIDC auth provider
*   Fix #2788: Return the correct file IDs on public link resources
*   Fix #2322: Use RFC3339 for parsing dates
*   Fix #2784: Disable storageprovider cache for the share jail
*   Fix #2555: Fix site accounts endpoints
*   Fix #2675: Updates Makefile according to latest go standards
*   Fix #2572: Wait for nats server on middleware start
*   Chg #2735: Avoid user enumeration
*   Chg #2737: Bump go-cs3api
*   Chg #2816: Merge the utility functions for space ids/references into one package
*   Chg #2763: Change the oCIS and S3NG  storage driver blob store layout
*   Chg #2596: Remove hash from public link urls
*   Chg #2785: Implement workaround for chi.RegisterMethod
*   Chg #2559: Do not encode webDAV ids to base64
*   Chg #2740: Rename oc10 share manager driver
*   Chg #2561: Merge oidcmapping auth manager into oidc
*   Enh #2698: Make capabilities endpoint public, authenticate users is present
*   Enh #2515: Enabling tracing by default if not explicitly disabled
*   Enh #2686: Features for favorites xattrs in EOS, cache for scope expansion
*   Enh #2494: Use sys ACLs for file permissions
*   Enh #2522: Introduce events
*   Enh #2811: Add event for created directories
*   Enh #2798: Add additional fields to events to enable search
*   Enh #2790: Fake providerids so API stays stable after beta
*   Enh #2685: Enable federated account access
*   Enh #1787: Add support for HTTP TPC
*   Enh #2799: Add flag to enable unrestriced listing of spaces
*   Enh #2560: Mentix PromSD extensions
*   Enh #2741: Meta path for user
*   Enh #2613: Externalize custom mime types configuration for storage providers
*   Enh #2163: Nextcloud-based share manager for pkg/ocm/share
*   Enh #2696: Preferences driver refactor and cbox sql implementation
*   Enh #2052: New CS3API datatx methods
*   Enh #2743: Add capability for public link single file edit
*   Enh #2738: Site accounts site-global settings
*   Enh #2672: Further Site Accounts improvements
*   Enh #2549: Site accounts improvements
*   Enh #2795: Add feature flags "projects" and "share_jail" to spaces capability
*   Enh #2514: Reuse ocs role objects in other drivers
*   Enh #2781: In memory user provider
*   Enh #2752: Refactor the rest user and group provider drivers

Details
-------

*   Bugfix #2693: Support editnew actions from MS Office

   This fixes the incorrect behavior when creating new xlsx and pptx files, as MS Office supports
   the editnew action and it must be used for newly created files instead of the normal edit action.

   https://github.com/cs3org/reva/pull/2693

*   Bugfix #2588: Dockerfile.revad-ceph to use the right base image

   In Aug2021 https://hub.docker.com/r/ceph/daemon-base was moved to quay.ceph.io and the
   builds for this image were failing for some weeks after January.

   https://github.com/cs3org/reva/pull/2588

*   Bugfix #2499: Removed check DenyGrant in resource permission

   When adding a denial permission

   https://github.com/cs3org/reva/pull/2499

*   Bugfix #2285: Accept new userid idp format

   The format for userid idp [changed](https://github.com/cs3org/cs3apis/pull/159) and
   this broke [the ocmd
   tutorial](https://reva.link/docs/tutorials/share-tutorial/#5-1-4-create-the-share)
   This PR makes the provider authorizer interceptor accept both the old and the new string
   format.

   https://github.com/cs3org/reva/issues/2285
   https://github.com/cs3org/reva/issues/2285
   See
   and

*   Bugfix #2802: Fix the resource id handling for space shares

   Adapt the space shares to the new id format.

   https://github.com/cs3org/reva/pull/2802

*   Bugfix #2800: Fix spaceid parsing in spaces trashbin API

   Added proper space id parsing to the spaces trashbin API endpoint.

   https://github.com/cs3org/reva/pull/2800

*   Bugfix #1846: Fix trash-bin propfind responses

   Fixed the href of the root element in trash-bin propfind responses.

   https://github.com/owncloud/ocis/issues/1846
   https://github.com/cs3org/reva/pull/2821

*   Bugfix #2608: Respect the tracing_service_name config variable

   https://github.com/cs3org/reva/pull/2608

*   Bugfix #2742: Use exact match in login filter

   After the recent config changes the auth-provider was accidently using a substring match for
   the login filter. It's no fixed to use an exact match.

   https://github.com/cs3org/reva/pull/2742

*   Bugfix #2759: Made uid, gid claims parsing more robust in OIDC auth provider

   This fix makes sure the uid and gid claims are defined at init time, and that the necessary
   typecasts are performed correctly when authenticating users. A comment was added that in case
   the uid/gid claims are missing AND that no mapping takes place, a user entity is returned with
   uid = gid = 0.

   https://github.com/cs3org/reva/pull/2759

*   Bugfix #2788: Return the correct file IDs on public link resources

   Resources in public shares should return the real resourceids from the storage of the owner.

   https://github.com/cs3org/reva/pull/2788

*   Bugfix #2322: Use RFC3339 for parsing dates

   We have used the RFC3339 format for parsing dates to be consistent with oC Web.

   https://github.com/cs3org/reva/issues/2322
   https://github.com/cs3org/reva/pull/2744

*   Bugfix #2784: Disable storageprovider cache for the share jail

   The share jail should not be cached in the provider cache because it is a virtual collection of
   resources from different storage providers.

   https://github.com/cs3org/reva/pull/2784

*   Bugfix #2555: Fix site accounts endpoints

   This PR fixes small bugs in the site accounts endpoints.

   https://github.com/cs3org/reva/pull/2555

*   Bugfix #2675: Updates Makefile according to latest go standards

   Earlier, we were using go get to install packages. Now, we are using go install to install
   packages

   https://github.com/cs3org/reva/issues/2675
   https://github.com/cs3org/reva/pull/2747

*   Bugfix #2572: Wait for nats server on middleware start

   Use a retry mechanism to connect to the nats server when it is not ready yet

   https://github.com/cs3org/reva/pull/2572

*   Change #2735: Avoid user enumeration

   Sending PROPFIND requests to `../files/admin` did return a different response than sending
   the same request to `../files/notexists`. This allowed enumerating users. This response was
   changed to be the same always

   https://github.com/cs3org/reva/pull/2735

*   Change #2737: Bump go-cs3api

   Bumped version of the go-cs3api

   https://github.com/cs3org/reva/pull/2737

*   Change #2816: Merge the utility functions for space ids/references into one package

   Merged the utility functions regarding space ids or references into one package. Also updated
   the functions to support the newly added provider id in the spaces id.

   https://github.com/cs3org/reva/pull/2816

*   Change #2763: Change the oCIS and S3NG  storage driver blob store layout

   We've optimized the oCIS and S3NG storage driver blob store layout.

   For the oCIS storage driver, blobs will now be stored inside the folder of a space, next to the
   nodes. This allows admins to easily archive, backup and restore spaces as a whole with UNIX
   tooling. We also moved from a single folder for blobs to multiple folders for blobs, to make the
   filesystem interactions more performant for large numbers of files.

   The previous layout on disk looked like this:

   ```markdown |-- spaces | |-- .. | | |-- .. | |-- xx | |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <-
   partitioned space id | |-- nodes | |-- .. | |-- xx | |-- xx | |-- xx | |-- xx | |--
   -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id |-- blobs |-- .. |--
   xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id ```

   Now it looks like this:

   ```markdown |-- spaces | |-- .. | | |-- .. |-- xx |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <-
   partitioned space id |-- nodes | |-- .. | |-- xx | |-- xx | |-- xx | |-- xx | |--
   -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id |-- blobs |-- .. |-- xx |-- xx |-- xx |-- xx
   |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id ```

   For the S3NG storage driver, blobs will now be prefixed with the space id and also a part of the
   blob id will be used as prefix. This creates a better prefix partitioning and mitigates S3 api
   performance drops for large buckets
   (https://aws.amazon.com/de/premiumsupport/knowledge-center/s3-prefix-nested-folders-difference/).

   The previous S3 bucket (blobs only looked like this):

   ```markdown |-- .. |-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id ```

   Now it looks like this:

   ```markdown |-- .. |-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- space id |-- .. |-- xx |-- xx
   |-- xx |-- xx |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id ```

   https://github.com/owncloud/ocis/issues/3557
   https://github.com/cs3org/reva/pull/2763

*   Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

*   Change #2785: Implement workaround for chi.RegisterMethod

   Implemented a workaround for `chi.RegisterMethod` because of a concurrent map read write
   issue. This needs to be fixed upstream in go-chi.

   https://github.com/cs3org/reva/pull/2785

*   Change #2559: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2559

*   Change #2740: Rename oc10 share manager driver

   We aligned the oc10 SQL share manager driver name with all other owncloud spacific SQL drivers
   by renaming the package `pkg/share/manager/sql` to `pkg/share/manager/owncloudsql` and
   changing the name from `oc10-sql` to `owncloudsql`.

   https://github.com/cs3org/reva/pull/2740

*   Change #2561: Merge oidcmapping auth manager into oidc

   The oidcmapping auth manager was created as a separate package to ease testing. As it has now
   been tested also as a pure OIDC auth provider without mapping, and as the code is largely
   refactored, it makes sense to merge it back so to maintain a single OIDC manager.

   https://github.com/cs3org/reva/pull/2561

*   Enhancement #2698: Make capabilities endpoint public, authenticate users is present

   https://github.com/cs3org/reva/pull/2698

*   Enhancement #2515: Enabling tracing by default if not explicitly disabled

   https://github.com/cs3org/reva/pull/2515

*   Enhancement #2686: Features for favorites xattrs in EOS, cache for scope expansion

   https://github.com/cs3org/reva/pull/2686

*   Enhancement #2494: Use sys ACLs for file permissions

   https://github.com/cs3org/reva/pull/2494

*   Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

*   Enhancement #2811: Add event for created directories

   We added another event for created directories.

   https://github.com/cs3org/reva/pull/2811

*   Enhancement #2798: Add additional fields to events to enable search

   https://github.com/cs3org/reva/pull/2798

*   Enhancement #2790: Fake providerids so API stays stable after beta

   To support the stativ registry, we need to accept providerids This fakes the ids so the API can
   stay stable

   https://github.com/cs3org/reva/pull/2790

*   Enhancement #2685: Enable federated account access

   https://github.com/cs3org/reva/pull/2685

*   Enhancement #1787: Add support for HTTP TPC

   We have added support for HTTP Third Party Copy. This allows remote data transfers between
   storages managed by either two different reva servers, or a reva server and a Grid
   (WLCG/ESCAPE) site server.

   Such remote transfers are expected to be driven by
   [GFAL](https://cern.ch/dmc-docs/gfal2/gfal2.html), the underlying library used by
   [FTS](https://cern.ch/fts), and [Rucio](https://rucio.cern.ch).

   In addition, the oidcmapping package has been refactored to support the standard OIDC use
   cases as well when no mapping is defined.

   https://github.com/cs3org/reva/issues/1787
   https://github.com/cs3org/reva/pull/2007

*   Enhancement #2799: Add flag to enable unrestriced listing of spaces

   Listing spaces now only returns all spaces when the user has the permissions and it was
   explicitly requested. The default will only return the spaces the current user has access to.

   https://github.com/cs3org/reva/pull/2799

*   Enhancement #2560: Mentix PromSD extensions

   The Mentix Prometheus SD scrape targets are now split into one file per service type, making
   health checks configuration easier. Furthermore, the local file connector for mesh data and
   the site registration endpoint have been dropped, as they aren't needed anymore.

   https://github.com/cs3org/reva/pull/2560

*   Enhancement #2741: Meta path for user

   We've added support for requesting the `meta-path-for-user` via a propfind to the
   `dav/meta/<id>` endpoint.

   https://github.com/cs3org/reva/pull/2741
   https://github.com/cs3org/reva/pull/2793
   https://doc.owncloud.com/server/next/developer_manual/webdav_api/meta.html

*   Enhancement #2613: Externalize custom mime types configuration for storage providers

   Added ability to configure custom mime types in an external JSON file, such that it can be reused
   when many storage providers are deployed at the same time.

   https://github.com/cs3org/reva/pull/2613

*   Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

*   Enhancement #2696: Preferences driver refactor and cbox sql implementation

   This PR uses the updated CS3APIs which accepts a namespace in addition to a single string key to
   recognize a user preference. It also refactors the GRPC service to support multiple drivers
   and adds the cbox SQL implementation.

   https://github.com/cs3org/reva/pull/2696

*   Enhancement #2052: New CS3API datatx methods

   CS3 datatx pull model methods: PullTransfer, RetryTransfer, ListTransfers Method
   CreateTransfer removed.

   https://github.com/cs3org/reva/pull/2052

*   Enhancement #2743: Add capability for public link single file edit

   It is now possible to share a single file by link with edit permissions. Therefore we need a
   public share capability to enable that feature in the clients. At the same time we improved the
   WebDAV permissions for public links.

   https://github.com/cs3org/reva/pull/2743

*   Enhancement #2738: Site accounts site-global settings

   This PR extends the site accounts service by adding site-global settings. These are used to
   store test user credentials that are in return used by our BBE port to perform CS3API-specific
   health checks.

   https://github.com/cs3org/reva/pull/2738

*   Enhancement #2672: Further Site Accounts improvements

   Yet another PR to update the site accounts (and Mentix): New default site ID; Include service
   type in alerts; Naming unified; Remove obsolete stuff.

   https://github.com/cs3org/reva/pull/2672

*   Enhancement #2549: Site accounts improvements

   This PR improves the site accounts: - Removed/hid API key stuff - Added quick links to the main
   panel - Made alert notifications mandatory

   https://github.com/cs3org/reva/pull/2549

*   Enhancement #2795: Add feature flags "projects" and "share_jail" to spaces capability

   https://github.com/cs3org/reva/pull/2795

*   Enhancement #2514: Reuse ocs role objects in other drivers

   https://github.com/cs3org/reva/pull/2514

*   Enhancement #2781: In memory user provider

   We added an in memory implementation for the user provider that reads the users from the
   mapstructure passed in.

   https://github.com/cs3org/reva/pull/2781

*   Enhancement #2752: Refactor the rest user and group provider drivers

   We now maintain our own cache for all user and group data, and periodically refresh it. A redis
   server now becomes a necessary dependency, whereas it was optional previously.

   https://github.com/cs3org/reva/pull/2752

Changelog for reva 2.2.0 (2022-04-12)
=======================================

The following sections list the changes in reva 2.2.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3373: Fix the permissions attribute in propfind responses
*   Fix #2721: Fix locking and public link scope checker to make the WOPI server work
*   Fix #2668: Minor cleanup
*   Fix #2692: Ensure that the host in the ocs config endpoint has no protocol
*   Fix #2709: Decomposed FS: return precondition failed if already locked
*   Chg #2687: Allow link with no or edit permission
*   Chg #2658: Small clean up of the ocdav code
*   Enh #2691: Decomposed FS: return a reference to the parent
*   Enh #2708: Rework LDAP configuration of user and group providers
*   Enh #2665: Add embeddable ocdav go micro service
*   Enh #2715: Introduced quicklinks
*   Enh #3370: Enable all spaces members to list public shares
*   Enh #3370: Enable space members to list shares inside the space
*   Enh #2717: Add definitions for user and group events

Details
-------

*   Bugfix #3373: Fix the permissions attribute in propfind responses

   Fixed the permissions that are returned when doing a propfind on a project space.

   https://github.com/owncloud/ocis/issues/3373
   https://github.com/cs3org/reva/pull/2713

*   Bugfix #2721: Fix locking and public link scope checker to make the WOPI server work

   We've fixed the locking implementation to use the CS3api instead of the temporary opaque
   values. We've fixed the scope checker on public links to allow the OpenInApp actions.

   These fixes have been done to use the cs3org/wopiserver with REVA edge.

   https://github.com/cs3org/reva/pull/2721

*   Bugfix #2668: Minor cleanup

   - The `chunk_folder` config option is unused - Prevent a panic when looking up spaces

   https://github.com/cs3org/reva/pull/2668

*   Bugfix #2692: Ensure that the host in the ocs config endpoint has no protocol

   We've fixed the host info in the ocs config endpoint so that it has no protocol, as ownCloud 10
   doesn't have it.

   https://github.com/cs3org/reva/pull/2692
   https://github.com/owncloud/ocis/pull/3113

*   Bugfix #2709: Decomposed FS: return precondition failed if already locked

   We've fixed the return code from permission denied to precondition failed if a user tries to
   lock an already locked file.

   https://github.com/cs3org/reva/pull/2709

*   Change #2687: Allow link with no or edit permission

   Allow the creation of links with no permissions. These can be used to navigate to a file that a
   user has access to. Allow setting edit permission on single file links (create and delete are
   still blocked) Introduce endpoint to get information about a given token

   https://github.com/cs3org/reva/pull/2687

*   Change #2658: Small clean up of the ocdav code

   Cleaned up the ocdav code to make it more readable and in one case a bit faster.

   https://github.com/cs3org/reva/pull/2658

*   Enhancement #2691: Decomposed FS: return a reference to the parent

   We've implemented the changes from cs3org/cs3apis#167 in the DecomposedFS, so that a stat on a
   resource always includes a reference to the parent of the resource.

   https://github.com/cs3org/reva/pull/2691

*   Enhancement #2708: Rework LDAP configuration of user and group providers

   We reworked to LDAP configuration of the LDAP user and group provider to share a common
   configuration scheme. Additionally the LDAP configuration no longer relies on templating
   LDAP filters in the configuration which is error prone and can be confusing. Additionally the
   providers are now somewhat more flexible about the group membership schema. Instead of only
   supporting RFC2307 (posixGroup) style groups. It's now possible to also use standard LDAP
   groups (groupOfName/groupOfUniqueNames) which track group membership by DN instead of
   username (the behaviour is switched automatically depending on the group_objectclass
   setting).

   The new LDAP configuration basically looks this:

   ```ini [grpc.services.userprovider.drivers.ldap] uri="ldaps://localhost:636"
   insecure=true user_base_dn="ou=testusers,dc=owncloud,dc=com"
   group_base_dn="ou=testgroups,dc=owncloud,dc=com" user_filter=""
   user_objectclass="posixAccount" group_filter="" group_objectclass="posixGroup"
   bind_username="cn=admin,dc=owncloud,dc=com" bind_password="admin"
   idp="http://localhost:20080"

   [grpc.services.userprovider.drivers.ldap.user_schema] id="entryuuid"
   displayName="displayName" userName="cn"

   [grpc.services.userprovider.drivers.ldap.group_schema] id="entryuuid"
   displayName="cn" groupName="cn" member="memberUID" ```

   `uri` defines the LDAP URI of the destination Server

   `insecure` allows to disable TLS Certifictate Validation (for development setups)

   `user_base_dn`/`group_base_dn` define the search bases for users and groups

   `user_filter`/`group_filter` allow to define additional LDAP filter of users and groups.
   This could be e.g. `(objectclass=owncloud)` to match for an additional objectclass.

   `user_objectclass`/`group_objectclass` define the main objectclass of Users and Groups.
   These are used to construct the LDAP filters

   `bind_username`/`bind_password` contain the authentication information for the LDAP
   connections

   The `user_schema` and `group_schema` sections define the mapping from CS3 user/group
   attributes to LDAP Attributes

   https://github.com/cs3org/reva/issues/2122
   https://github.com/cs3org/reva/issues/2124
   https://github.com/cs3org/reva/pull/2708

*   Enhancement #2665: Add embeddable ocdav go micro service

   The new `pkg/micro/ocdav` package implements a go micro compatible version of the ocdav
   service.

   https://github.com/cs3org/reva/pull/2665

*   Enhancement #2715: Introduced quicklinks

   We now support Quicklinks. When creating a link with flag "quicklink=true", no new link will be
   created when a link already exists.

   https://github.com/cs3org/reva/pull/2715

*   Enhancement #3370: Enable all spaces members to list public shares

   Enhanced the json and cs3 public share manager so that it lists shares in spaces for all members.

   https://github.com/owncloud/ocis/issues/3370
   https://github.com/cs3org/reva/pull/2697

*   Enhancement #3370: Enable space members to list shares inside the space

   If there are shared resources in a space then all members are allowed to see those shares. The
   json share manager was enhanced to check if the user is allowed to see a share by checking the
   grants on a resource.

   https://github.com/owncloud/ocis/issues/3370
   https://github.com/cs3org/reva/pull/2674
   https://github.com/cs3org/reva/pull/2710

*   Enhancement #2717: Add definitions for user and group events

   Enhance the events package with definitions for user and group events.

   https://github.com/cs3org/reva/pull/2717
   https://github.com/cs3org/reva/pull/2724

Changelog for reva 2.1.0 (2022-03-29)
=======================================

The following sections list the changes in reva 2.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2636: Delay reconnect log for events
*   Fix #2645: Avoid warning about missing .flock files
*   Fix #2625: Fix locking on publik links and the decomposed filesystem
*   Fix #2643: Emit linkaccessfailed event when share is nil
*   Fix #2646: Replace public mountpoint fileid with grant fileid in ocdav
*   Fix #2612: Adjust the scope handling to support the spaces architecture
*   Fix #2621: Send events only if response code is `OK`
*   Chg #2574: Switch NATS backend
*   Chg #2667: Allow LDAP groups to have no gidNumber
*   Chg #3233: Improve quota handling
*   Chg #2600: Use the cs3 share api to manage spaces
*   Enh #2644: Add new public share manager
*   Enh #2626: Add new share manager
*   Enh #2624: Add etags to virtual spaces
*   Enh #2639: File Events
*   Enh #2627: Add events for sharing action
*   Enh #2664: Add grantID to mountpoint
*   Enh #2622: Allow listing shares in spaces via the OCS API
*   Enh #2623: Add space aliases
*   Enh #2647: Add space specific events
*   Enh #3345: Add the spaceid to propfind responses
*   Enh #2616: Add etag to spaces response
*   Enh #2628: Add spaces aware trash-bin API

Details
-------

*   Bugfix #2636: Delay reconnect log for events

   Print reconnect information log only when reconnect time is bigger than a second

   https://github.com/cs3org/reva/pull/2636

*   Bugfix #2645: Avoid warning about missing .flock files

   These flock files appear randomly because of file locking. We can savely ignore them.

   https://github.com/cs3org/reva/pull/2645

*   Bugfix #2625: Fix locking on publik links and the decomposed filesystem

   We've fixed the behavior of locking on the decomposed filesystem, so that now app based locks
   can be modified user independently (needed for WOPI integration). Also we added a check, if a
   lock is already expired and if so, we lazily delete the lock. The InitiateUploadRequest now
   adds the Lock to the upload metadata so that an upload to an locked file is possible.

   We'v added the locking api requests to the public link scope checks, so that locking also can be
   used on public links (needed for WOPI integration).

   https://github.com/cs3org/reva/pull/2625

*   Bugfix #2643: Emit linkaccessfailed event when share is nil

   The code no longer panics when a link access failed event has no share.

   https://github.com/cs3org/reva/pull/2643

*   Bugfix #2646: Replace public mountpoint fileid with grant fileid in ocdav

   We now show the same resoucre id for resources when accessing them via a public links as when
   using a logged in user. This allows the web ui to start a WOPI session with the correct resource
   id.

   https://github.com/cs3org/reva/issues/2635
   https://github.com/cs3org/reva/pull/2646

*   Bugfix #2612: Adjust the scope handling to support the spaces architecture

   The scope authentication interceptors weren't updated to the spaces architecture and
   couldn't authenticate some requests.

   https://github.com/cs3org/reva/pull/2612

*   Bugfix #2621: Send events only if response code is `OK`

   Before events middleware was sending events also when the resulting status code was not `OK`.
   This is clearly a bug.

   https://github.com/cs3org/reva/pull/2621

*   Change #2574: Switch NATS backend

   We've switched the NATS backend from Streaming to JetStream, since NATS Streaming is
   depreciated.

   https://github.com/cs3org/reva/pull/2574

*   Change #2667: Allow LDAP groups to have no gidNumber

   Similar to the user-provider allow a group to have no gidNumber. Assign a default in that case.

   https://github.com/cs3org/reva/pull/2667

*   Change #3233: Improve quota handling

   GetQuota now returns 0 when no quota was set instead of the disk size. Also added a new return
   value for the remaining space which will either be quota - used bytes or if no quota was set the
   free disk size.

   https://github.com/owncloud/ocis/issues/3233
   https://github.com/cs3org/reva/pull/2666
   https://github.com/cs3org/reva/pull/2688

*   Change #2600: Use the cs3 share api to manage spaces

   We now use the cs3 share Api to manage the space roles. We do not send the request to the share
   manager, the permissions are stored in the storage provider

   https://github.com/cs3org/reva/pull/2600
   https://github.com/cs3org/reva/pull/2620
   https://github.com/cs3org/reva/pull/2687

*   Enhancement #2644: Add new public share manager

   We added a new public share manager which uses the new metadata storage backend for persisting
   the public share information.

   https://github.com/cs3org/reva/pull/2644

*   Enhancement #2626: Add new share manager

   We added a new share manager which uses the new metadata storage backend for persisting the
   share information.

   https://github.com/cs3org/reva/pull/2626

*   Enhancement #2624: Add etags to virtual spaces

   The shares storage provider didn't include the etag in virtual spaces like the shares jail or
   mountpoints.

   https://github.com/cs3org/reva/pull/2624

*   Enhancement #2639: File Events

   Adds file based events. See `pkg/events/files.go` for full list

   https://github.com/cs3org/reva/pull/2639

*   Enhancement #2627: Add events for sharing action

   Includes lifecycle events for shares and public links doesn't include federated sharing
   events for now see full list of events in `pkg/events/types.go`

   https://github.com/cs3org/reva/pull/2627

*   Enhancement #2664: Add grantID to mountpoint

   We distinguish between the mountpoint of a share and the grant where the original file is
   located on the storage.

   https://github.com/cs3org/reva/pull/2664

*   Enhancement #2622: Allow listing shares in spaces via the OCS API

   Added a `space_ref` parameter to the list shares endpoints so that one can list shares inside of
   spaces.

   https://github.com/cs3org/reva/pull/2622

*   Enhancement #2623: Add space aliases

   Space aliases can be used to resolve spaceIDs in a client.

   https://github.com/cs3org/reva/pull/2623

*   Enhancement #2647: Add space specific events

   See `pkg/events/spaces.go` for full list

   https://github.com/cs3org/reva/pull/2647

*   Enhancement #3345: Add the spaceid to propfind responses

   Added the spaceid to propfind responses so that clients have the necessary data to send
   subsequent requests.

   https://github.com/owncloud/ocis/issues/3345
   https://github.com/cs3org/reva/pull/2657

*   Enhancement #2616: Add etag to spaces response

   Added the spaces etag to the response when listing spaces.

   https://github.com/cs3org/reva/pull/2616

*   Enhancement #2628: Add spaces aware trash-bin API

   Added the webdav trash-bin endpoint for spaces.

   https://github.com/cs3org/reva/pull/2628

Changelog for reva 2.0.0 (2022-03-03)
=======================================

The following sections list the changes in reva 2.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2457: Do not swallow error
*   Fix #2422: Handle non existing spaces correctly
*   Fix #2327: Enable changelog on edge branch
*   Fix #2370: Fixes for apps in public shares, project spaces for EOS driver
*   Fix #2464: Pass spacegrants when adding member to space
*   Fix #2430: Fix aggregated child folder id
*   Fix #2348: Make archiver handle spaces protocol
*   Fix #2452: Fix create space error message
*   Fix #2445: Don't handle ids containing "/" in decomposedfs
*   Fix #2285: Accept new userid idp format
*   Fix #2503: Remove the protection from /v?.php/config endpoints
*   Fix #2462: Public shares path needs to be set
*   Fix #2427: Fix registry caching
*   Fix #2298: Remove share refs from trashbin
*   Fix #2433: Fix shares provider filter
*   Fix #2351: Fix Statcache removing
*   Fix #2374: Fix webdav copy of zero byte files
*   Fix #2336: Handle sending all permissions when creating public links
*   Fix #2440: Add ArbitraryMetadataKeys to statcache key
*   Fix #2582: Keep lock structs in a local map protected by a mutex
*   Fix #2372: Make owncloudsql work with the spaces registry
*   Fix #2416: The registry now returns complete space structs
*   Fix #3066: Fix propfind listing for files
*   Fix #2428: Remove unused home provider from config
*   Fix #2334: Revert fix decomposedfs upload
*   Fix #2415: Services should never return transport level errors
*   Fix #2419: List project spaces for share recipients
*   Fix #2501: Fix spaces stat
*   Fix #2432: Use space reference when listing containers
*   Fix #2572: Wait for nats server on middleware start
*   Fix #2454: Fix webdav paths in PROPFINDS
*   Chg #2329: Activate the statcache
*   Chg #2596: Remove hash from public link urls
*   Chg #2495: Remove the ownCloud storage driver
*   Chg #2527: Store space attributes in decomposedFS
*   Chg #2581: Update hard-coded status values
*   Chg #2524: Use description during space creation
*   Chg #2554: Shard nodes per space in decomposedfs
*   Chg #2576: Harden xattrs errors
*   Chg #2436: Replace template in GroupFilter for UserProvider with a simple string
*   Chg #2429: Make archiver id based
*   Chg #2340: Allow multiple space configurations per provider
*   Chg #2396: The ocdav handler is now spaces aware
*   Chg #2349: Require `ListRecycle` when listing trashbin
*   Chg #2353: Reduce log output
*   Chg #2542: Do not encode webDAV ids to base64
*   Chg #2519: Remove the auto creation of the .space folder
*   Chg #2394: Remove logic from gateway
*   Chg #2023: Add a sharestorageprovider
*   Chg #2234: Add a spaces registry
*   Chg #2339: Fix static registry regressions
*   Chg #2370: Fix static registry regressions
*   Chg #2354: Return not found when updating non existent space
*   Chg #2589: Remove deprecated linter modules
*   Chg #2016: Move wrapping and unwrapping of paths to the storage gateway
*   Enh #2591: Set up App Locks with basic locks
*   Enh #1209: Reva CephFS module v0.2.1
*   Enh #2511: Error handling cleanup in decomposed FS
*   Enh #2516: Cleaned up some code
*   Enh #2512: Consolidate xattr setter and getter
*   Enh #2341: Use CS3 permissions API
*   Enh #2343: Allow multiple space type fileters on decomposedfs
*   Enh #2460: Add locking support to decomposedfs
*   Enh #2540: Refactored the xattrs package in the decomposedfs
*   Enh #2463: Do not log whole nodes
*   Enh #2350: Add file locking methods to the storage and filesystem interfaces
*   Enh #2379: Add new file url of the app provider to the ocs capabilities
*   Enh #2369: Implement TouchFile from the CS3apis
*   Enh #2385: Allow to create new files with the app provider on public links
*   Enh #2397: Product field in OCS version
*   Enh #2393: Update tus/tusd to version 1.8.0
*   Enh #2522: Introduce events
*   Enh #2528: Use an exclcusive write lock when writing multiple attributes
*   Enh #2595: Add integration test for the groupprovider
*   Enh #2439: Ignore handled errors when creating spaces
*   Enh #2500: Invalidate listproviders cache
*   Enh #2345: Don't assume that the LDAP groupid in reva matches the name
*   Enh #2525: Allow using AD UUID as userId values
*   Enh #2584: Allow running userprovider integration tests for the LDAP driver
*   Enh #2585: Add metadata storage layer and indexer
*   Enh #2163: Nextcloud-based share manager for pkg/ocm/share
*   Enh #2278: OIDC driver changes for lightweight users
*   Enh #2315: Add new attributes to public link propfinds
*   Enh #2431: Delete shares when purging spaces
*   Enh #2434: Refactor ocdav into smaller chunks
*   Enh #2524: Add checks when removing space members
*   Enh #2457: Restore spaces that were previously deleted
*   Enh #2498: Include grants in list storage spaces response
*   Enh #2344: Allow listing all storage spaces
*   Enh #2547: Add an if-match check to the storage provider
*   Enh #2486: Update cs3apis to include lock api changes
*   Enh #2526: Upgrade ginkgo to v2

Details
-------

*   Bugfix #2457: Do not swallow error

   Decomposedfs not longer swallows errors when creating a node fails.

   https://github.com/cs3org/reva/pull/2457

*   Bugfix #2422: Handle non existing spaces correctly

   When looking up a space by id we returned the wrong status code.

   https://github.com/cs3org/reva/pull/2422

*   Bugfix #2327: Enable changelog on edge branch

   We added a `branch` flag to the `tools/check-changelog/main.go` to fix changelog checks on
   the edge branch.

   https://github.com/cs3org/reva/pull/2327

*   Bugfix #2370: Fixes for apps in public shares, project spaces for EOS driver

   https://github.com/cs3org/reva/pull/2370

*   Bugfix #2464: Pass spacegrants when adding member to space

   When creating a space grant there should not be created a new space. Unfortunately SpaceGrant
   didn't work when adding members to a space. Now a value is placed in the ctx of the
   storageprovider on which decomposedfs reacts

   https://github.com/cs3org/reva/pull/2464

*   Bugfix #2430: Fix aggregated child folder id

   Propfind now returns the correct id and correctly aggregates the mtime and etag.

   https://github.com/cs3org/reva/pull/2430

*   Bugfix #2348: Make archiver handle spaces protocol

   The archiver can now handle the spaces protocol

   https://github.com/cs3org/reva/pull/2348

*   Bugfix #2452: Fix create space error message

   Create space no longer errors with list spaces error messages.

   https://github.com/cs3org/reva/pull/2452

*   Bugfix #2445: Don't handle ids containing "/" in decomposedfs

   The storageprovider previously checked all ids without checking their validity this lead to
   flaky test because it shouldn't check ids that are used from the public storage provider

   https://github.com/cs3org/reva/pull/2445

*   Bugfix #2285: Accept new userid idp format

   The format for userid idp [changed](https://github.com/cs3org/cs3apis/pull/159) and
   this broke [the ocmd
   tutorial](https://reva.link/docs/tutorials/share-tutorial/#5-1-4-create-the-share)
   This PR makes the provider authorizer interceptor accept both the old and the new string
   format.

   https://github.com/cs3org/reva/issues/2285
   https://github.com/cs3org/reva/issues/2285
   See
   and

*   Bugfix #2503: Remove the protection from /v?.php/config endpoints

   We've removed the protection from the "/v1.php/config" and "/v2.php/config" endpoints to be
   API compatible with ownCloud 10.

   https://github.com/cs3org/reva/issues/2503
   https://github.com/owncloud/ocis/issues/1338

*   Bugfix #2462: Public shares path needs to be set

   We need to set the relative path to the space root for public link shares to identify them in the
   shares list.

   https://github.com/owncloud/ocis/issues/2462
   https://github.com/cs3org/reva/pull/2580

*   Bugfix #2427: Fix registry caching

   We now cache space lookups per user.

   https://github.com/cs3org/reva/pull/2427

*   Bugfix #2298: Remove share refs from trashbin

   https://github.com/cs3org/reva/pull/2298

*   Bugfix #2433: Fix shares provider filter

   The shares storage provider now correctly filters space types

   https://github.com/cs3org/reva/pull/2433

*   Bugfix #2351: Fix Statcache removing

   Removing from statcache didn't work correctly with different setups. Unified and fixed

   https://github.com/cs3org/reva/pull/2351

*   Bugfix #2374: Fix webdav copy of zero byte files

   We've fixed the webdav copy action of zero byte files, which was not performed because the
   webdav api assumed, that zero byte uploads are created when initiating the upload, which was
   recently removed from all storage drivers. Therefore the webdav api also uploads zero byte
   files after initiating the upload.

   https://github.com/cs3org/reva/pull/2374
   https://github.com/cs3org/reva/pull/2309

*   Bugfix #2336: Handle sending all permissions when creating public links

   For backwards compatability we now reduce permissions to readonly when a create public link
   carries all permissions.

   https://github.com/cs3org/reva/issues/2336
   https://github.com/owncloud/ocis/issues/1269

*   Bugfix #2440: Add ArbitraryMetadataKeys to statcache key

   Otherwise stating with and without them would return the same result (because it is cached)

   https://github.com/cs3org/reva/pull/2440

*   Bugfix #2582: Keep lock structs in a local map protected by a mutex

   Make sure that only one go routine or process can get the lock.

   https://github.com/cs3org/reva/pull/2582

*   Bugfix #2372: Make owncloudsql work with the spaces registry

   Owncloudsql now works properly with the spaces registry.

   https://github.com/cs3org/reva/pull/2372

*   Bugfix #2416: The registry now returns complete space structs

   We now return the complete space info, including name, path, owner, etc. instead of only path
   and id.

   https://github.com/cs3org/reva/pull/2416

*   Bugfix #3066: Fix propfind listing for files

   When doing a propfind for a file the result contained the files twice.

   https://github.com/owncloud/ocis/issues/3066
   https://github.com/cs3org/reva/pull/2506

*   Bugfix #2428: Remove unused home provider from config

   The spaces registry does not use a home provider config.

   https://github.com/cs3org/reva/pull/2428

*   Bugfix #2334: Revert fix decomposedfs upload

   Reverting https://github.com/cs3org/reva/pull/2330 to fix it properly

   https://github.com/cs3org/reva/pull/2334

*   Bugfix #2415: Services should never return transport level errors

   The CS3 API adopted the grpc error codes from the [google grpc status
   package](https://github.com/googleapis/googleapis/blob/master/google/rpc/status.proto).
   It also separates transport level errors from application level errors on purpose. This
   allows sending CS3 messages over protocols other than GRPC. To keep that seperation, the
   server side must always return `nil`, even though the code generation for go produces function
   signatures for rpcs with an `error` return property. That allows clients to clearly
   distinguish between transport level errors indicated by `err != nil` the error and
   application level errors by checking the status code.

   https://github.com/cs3org/reva/pull/2415

*   Bugfix #2419: List project spaces for share recipients

   The sharing handler now uses the ListProvider call on the registry when sharing by reference.
   Furthermore, the decomposedfs now checks permissions on the root of a space so that a space is
   listed for users that have access to a space.

   https://github.com/cs3org/reva/pull/2419

*   Bugfix #2501: Fix spaces stat

   When stating a space e.g. the Share Jail and that space contains another space, in this case a
   share then the stat would sometimes get the sub space instead of the Share Jail itself.

   https://github.com/cs3org/reva/pull/2501

*   Bugfix #2432: Use space reference when listing containers

   The propfind handler now uses the reference for a space to make lookups relative.

   https://github.com/cs3org/reva/pull/2432

*   Bugfix #2572: Wait for nats server on middleware start

   Use a retry mechanism to connect to the nats server when it is not ready yet

   https://github.com/cs3org/reva/pull/2572

*   Bugfix #2454: Fix webdav paths in PROPFINDS

   The WebDAV Api was handling paths on spaces propfinds in the wrong way. This has been fixed in the
   WebDAV layer.

   https://github.com/cs3org/reva/pull/2454

*   Change #2329: Activate the statcache

   Activates the cache of stat request/responses in the gateway.

   https://github.com/cs3org/reva/pull/2329

*   Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

*   Change #2495: Remove the ownCloud storage driver

   We've removed the ownCloud storage driver because it was no longer maintained after the
   ownCloud SQL storage driver was added.

   If you have been using the ownCloud storage driver, please switch to the ownCloud SQL storage
   driver which brings you more features and is under active maintenance.

   https://github.com/cs3org/reva/pull/2495

*   Change #2527: Store space attributes in decomposedFS

   We need to store more space attributes in the storage. This implements extended space
   attributes in the decomposedFS

   https://github.com/cs3org/reva/pull/2527

*   Change #2581: Update hard-coded status values

   The hard-coded version and product values have been updated to be consistent in all places in
   the code.

   https://github.com/cs3org/reva/pull/2581

*   Change #2524: Use description during space creation

   We can now use a space description during space creation. We also fixed a bug in the spaces roles.
   Co-owners are now maintainers.

   https://github.com/cs3org/reva/pull/2524

*   Change #2554: Shard nodes per space in decomposedfs

   The decomposedfs changas the on disk layout to shard nodes per space.

   https://github.com/cs3org/reva/pull/2554

*   Change #2576: Harden xattrs errors

   Unwrap the error to get the root error.

   https://github.com/cs3org/reva/pull/2576

*   Change #2436: Replace template in GroupFilter for UserProvider with a simple string

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

*   Change #2429: Make archiver id based

   The archiver now uses ids to walk the tree instead of paths

   https://github.com/cs3org/reva/pull/2429

*   Change #2340: Allow multiple space configurations per provider

   The spaces registry can now use multiple space configurations to allow personal and project
   spaces on the same provider

   https://github.com/cs3org/reva/pull/2340

*   Change #2396: The ocdav handler is now spaces aware

   It will use LookupStorageSpaces and make only relative requests to the gateway. Temp comment

   https://github.com/cs3org/reva/pull/2396

*   Change #2349: Require `ListRecycle` when listing trashbin

   Previously there was no check, so anyone could list anyones trash

   https://github.com/cs3org/reva/pull/2349

*   Change #2353: Reduce log output

   Reduced log output. Some errors or warnings were logged multiple times or even unnecesarily.

   https://github.com/cs3org/reva/pull/2353

*   Change #2542: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2542
   https://github.com/cs3org/reva/pull/2558

*   Change #2519: Remove the auto creation of the .space folder

   We removed the auto creation of the .space folder because we don't develop this feature
   further.

   https://github.com/cs3org/reva/pull/2519

*   Change #2394: Remove logic from gateway

   The gateway will now hold no logic except forwarding the requests to other services.

   https://github.com/cs3org/reva/pull/2394

*   Change #2023: Add a sharestorageprovider

   This PR adds a ShareStorageProvider which enables us to get rid of a lot of special casing in
   other parts of the code. It also fixes several issues regarding shares and group shares.

   https://github.com/cs3org/reva/pull/2023

*   Change #2234: Add a spaces registry

   Spaces registry is supposed to manage spaces. Read
   `pkg/storage/registry/spaces/Readme.md` for full details

   https://github.com/cs3org/reva/pull/2234

*   Change #2339: Fix static registry regressions

   We fixed some smaller issues with using the static registry which were introduced with the
   spaces registry changes.

   https://github.com/cs3org/reva/pull/2339

*   Change #2370: Fix static registry regressions

   We fixed some smaller issues with using the static registry which were introduced with the
   spaces registry changes.

   https://github.com/cs3org/reva/pull/2370

*   Change #2354: Return not found when updating non existent space

   If a spaceid of a space which is updated doesn't exist, handle it as a not found error.

   https://github.com/cs3org/reva/pull/2354

*   Change #2589: Remove deprecated linter modules

   Replaced the deprecated linter modules with the recommended ones.

   https://github.com/cs3org/reva/pull/2589

*   Change #2016: Move wrapping and unwrapping of paths to the storage gateway

   We've moved the wrapping and unwrapping of reference paths to the storage gateway so that the
   storageprovider doesn't have to know its mount path.

   https://github.com/cs3org/reva/pull/2016

*   Enhancement #2591: Set up App Locks with basic locks

   To set up App Locks basic locks are used now

   https://github.com/cs3org/reva/pull/2591

*   Enhancement #1209: Reva CephFS module v0.2.1

   https://github.com/cs3org/reva/pull/1209

*   Enhancement #2511: Error handling cleanup in decomposed FS

   - Avoid inconsensitencies by cleaning up actions in case of err

   https://github.com/cs3org/reva/pull/2511

*   Enhancement #2516: Cleaned up some code

   - Reduced type conversions []byte <-> string - pre-compile chunking regex

   https://github.com/cs3org/reva/pull/2516

*   Enhancement #2512: Consolidate xattr setter and getter

   - Consolidate all metadata Get's and Set's to central functions. - Cleaner code by reduction of
   casts - Easier to hook functionality like indexing

   https://github.com/cs3org/reva/pull/2512

*   Enhancement #2341: Use CS3 permissions API

   Added calls to the CS3 permissions API to the decomposedfs in order to check the user
   permissions.

   https://github.com/cs3org/reva/pull/2341

*   Enhancement #2343: Allow multiple space type fileters on decomposedfs

   The decomposedfs driver now evaluates multiple space type filters when listing storage
   spaces.

   https://github.com/cs3org/reva/pull/2343

*   Enhancement #2460: Add locking support to decomposedfs

   The decomposedfs now implements application level locking

   https://github.com/cs3org/reva/pull/2460

*   Enhancement #2540: Refactored the xattrs package in the decomposedfs

   The xattrs package now uses the xattr.ENOATTR instead of os.ENODATA or os.ENOATTR to check
   attribute existence.

   https://github.com/cs3org/reva/pull/2540
   https://github.com/cs3org/reva/pull/2541

*   Enhancement #2463: Do not log whole nodes

   It turns out that logging whole node objects is very expensive and also spams the logs quite a
   bit. Instead we just log the node ID now.

   https://github.com/cs3org/reva/pull/2463

*   Enhancement #2350: Add file locking methods to the storage and filesystem interfaces

   We've added the file locking methods from the CS3apis to the storage and filesystem
   interfaces. As of now they are dummy implementations and will only return "unimplemented"
   errors.

   https://github.com/cs3org/reva/pull/2350
   https://github.com/cs3org/cs3apis/pull/160

*   Enhancement #2379: Add new file url of the app provider to the ocs capabilities

   We've added the new file capability of the app provider to the ocs capabilities, so that clients
   can discover this url analogous to the app list and file open urls.

   https://github.com/cs3org/reva/pull/2379
   https://github.com/owncloud/ocis/pull/2884
   https://github.com/owncloud/web/pull/5890#issuecomment-993905242

*   Enhancement #2369: Implement TouchFile from the CS3apis

   We've updated the CS3apis and implemented the TouchFile method.

   https://github.com/cs3org/reva/pull/2369
   https://github.com/cs3org/cs3apis/pull/154

*   Enhancement #2385: Allow to create new files with the app provider on public links

   We've added the option to create files with the app provider on public links.

   https://github.com/cs3org/reva/pull/2385

*   Enhancement #2397: Product field in OCS version

   We've added a new field to the OCS Version, which is supposed to announce the product name. The
   web ui as a client will make use of it to make the backend product and version available (e.g. for
   easier bug reports).

   https://github.com/cs3org/reva/pull/2397

*   Enhancement #2393: Update tus/tusd to version 1.8.0

   We've update tus/tusd to version 1.8.0.

   https://github.com/cs3org/reva/issues/2393
   https://github.com/cs3org/reva/pull/2224

*   Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

*   Enhancement #2528: Use an exclcusive write lock when writing multiple attributes

   The xattr package can use an exclusive write lock when writing multiple extended attributes

   https://github.com/cs3org/reva/pull/2528

*   Enhancement #2595: Add integration test for the groupprovider

   Some new integration tests were added to cover the groupprovider.

   https://github.com/cs3org/reva/pull/2595

*   Enhancement #2439: Ignore handled errors when creating spaces

   The CreateStorageSpace no longer logs all error cases with error level logging

   https://github.com/cs3org/reva/pull/2439

*   Enhancement #2500: Invalidate listproviders cache

   We now invalidate the related listproviders cache entries when updating or deleting a storage
   space.

   https://github.com/cs3org/reva/pull/2500

*   Enhancement #2345: Don't assume that the LDAP groupid in reva matches the name

   This allows using attributes like e.g. `entryUUID` or any custom id attribute as the id for
   groups.

   https://github.com/cs3org/reva/pull/2345

*   Enhancement #2525: Allow using AD UUID as userId values

   Active Directory UUID attributes (like e.g. objectGUID) use the LDAP octectString Syntax. In
   order to be able to use them as userids in reva, they need to be converted to their string
   representation.

   https://github.com/cs3org/reva/pull/2525

*   Enhancement #2584: Allow running userprovider integration tests for the LDAP driver

   We extended the integration test suite for the userprovider to allow running it with an LDAP
   server.

   https://github.com/cs3org/reva/pull/2584

*   Enhancement #2585: Add metadata storage layer and indexer

   We ported over and enhanced the metadata storage layer and indexer from ocis-pkg so that it can
   be used by reva services as well.

   https://github.com/cs3org/reva/pull/2585

*   Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

*   Enhancement #2278: OIDC driver changes for lightweight users

   https://github.com/cs3org/reva/pull/2278

*   Enhancement #2315: Add new attributes to public link propfinds

   Added a new property "oc:signature-auth" to public link propfinds. This is a necessary change
   to be able to support archive downloads in password protected public links.

   https://github.com/cs3org/reva/pull/2315

*   Enhancement #2431: Delete shares when purging spaces

   Implemented the second step of the two step spaces delete process. The first step is marking the
   space as deleted, the second step is actually purging the space. During the second step all
   shares, including public shares, in the space will be deleted. When deleting a space the blobs
   are currently not yet deleted since the decomposedfs will receive some changes soon.

   https://github.com/cs3org/reva/pull/2431
   https://github.com/cs3org/reva/pull/2458

*   Enhancement #2434: Refactor ocdav into smaller chunks

   That increases code clarity and enables testing.

   https://github.com/cs3org/reva/pull/2434

*   Enhancement #2524: Add checks when removing space members

   - Removed owners from project spaces - Prevent deletion of last space manager - Viewers and
   editors can always be deleted - Managers can only be deleted when there will be at least one
   remaining

   https://github.com/cs3org/reva/pull/2524

*   Enhancement #2457: Restore spaces that were previously deleted

   After the first step of the two step delete process an admin can decide to restore the space
   instead of deleting it. This will undo the deletion and all files and shares are accessible
   again

   https://github.com/cs3org/reva/pull/2457

*   Enhancement #2498: Include grants in list storage spaces response

   Added the grants to the response of list storage spaces. This allows service clients to show who
   has access to a space.

   https://github.com/cs3org/reva/pull/2498

*   Enhancement #2344: Allow listing all storage spaces

   To implement the drives api we now list all spaces when no filter is given. The Provider info will
   not contain any spaces as the client is responsible for looking up the spaces.

   https://github.com/cs3org/reva/pull/2344

*   Enhancement #2547: Add an if-match check to the storage provider

   Implement a check for the if-match value in InitiateFileUpload to prevent overwrites of newer
   versions.

   https://github.com/cs3org/reva/pull/2547

*   Enhancement #2486: Update cs3apis to include lock api changes

   https://github.com/cs3org/reva/pull/2486

*   Enhancement #2526: Upgrade ginkgo to v2

   https://github.com/cs3org/reva/pull/2526

Changelog for reva 1.18.0 (2022-02-11)
=======================================

The following sections list the changes in reva 1.18.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2370: Fixes for apps in public shares, project spaces for EOS driver
*   Fix #2374: Fix webdav copy of zero byte files
*   Fix #2478: Use ocs permission objects in the reva GRPC client
*   Fix #2368: Return wrapped paths for recycled items in storage provider
*   Chg #2354: Return not found when updating non existent space
*   Enh #1209: Reva CephFS module v0.2.1
*   Enh #2341: Use CS3 permissions API
*   Enh #2350: Add file locking methods to the storage and filesystem interfaces
*   Enh #2379: Add new file url of the app provider to the ocs capabilities
*   Enh #2369: Implement TouchFile from the CS3apis
*   Enh #2385: Allow to create new files with the app provider on public links
*   Enh #2397: Product field in OCS version
*   Enh #2393: Update tus/tusd to version 1.8.0
*   Enh #2205: Modify group and user managers to skip fetching specified metadata
*   Enh #2232: Make ocs resource info cache interoperable across drivers
*   Enh #2233: Populate owner data in the ocs and ocdav services
*   Enh #2278: OIDC driver changes for lightweight users

Details
-------

*   Bugfix #2370: Fixes for apps in public shares, project spaces for EOS driver

   https://github.com/cs3org/reva/pull/2370

*   Bugfix #2374: Fix webdav copy of zero byte files

   We've fixed the webdav copy action of zero byte files, which was not performed because the
   webdav api assumed, that zero byte uploads are created when initiating the upload, which was
   recently removed from all storage drivers. Therefore the webdav api also uploads zero byte
   files after initiating the upload.

   https://github.com/cs3org/reva/pull/2374
   https://github.com/cs3org/reva/pull/2309

*   Bugfix #2478: Use ocs permission objects in the reva GRPC client

   There was a bug introduced by differing CS3APIs permission definitions for the same role
   across services. This is a first step in making all services use consistent definitions.

   https://github.com/cs3org/reva/pull/2478

*   Bugfix #2368: Return wrapped paths for recycled items in storage provider

   https://github.com/cs3org/reva/pull/2368

*   Change #2354: Return not found when updating non existent space

   If a spaceid of a space which is updated doesn't exist, handle it as a not found error.

   https://github.com/cs3org/reva/pull/2354

*   Enhancement #1209: Reva CephFS module v0.2.1

   https://github.com/cs3org/reva/pull/1209

*   Enhancement #2341: Use CS3 permissions API

   Added calls to the CS3 permissions API to the decomposedfs in order to check the user
   permissions.

   https://github.com/cs3org/reva/pull/2341

*   Enhancement #2350: Add file locking methods to the storage and filesystem interfaces

   We've added the file locking methods from the CS3apis to the storage and filesystem
   interfaces. As of now they are dummy implementations and will only return "unimplemented"
   errors.

   https://github.com/cs3org/reva/pull/2350
   https://github.com/cs3org/cs3apis/pull/160

*   Enhancement #2379: Add new file url of the app provider to the ocs capabilities

   We've added the new file capability of the app provider to the ocs capabilities, so that clients
   can discover this url analogous to the app list and file open urls.

   https://github.com/cs3org/reva/pull/2379
   https://github.com/owncloud/ocis/pull/2884
   https://github.com/owncloud/web/pull/5890#issuecomment-993905242

*   Enhancement #2369: Implement TouchFile from the CS3apis

   We've updated the CS3apis and implemented the TouchFile method.

   https://github.com/cs3org/reva/pull/2369
   https://github.com/cs3org/cs3apis/pull/154

*   Enhancement #2385: Allow to create new files with the app provider on public links

   We've added the option to create files with the app provider on public links.

   https://github.com/cs3org/reva/pull/2385

*   Enhancement #2397: Product field in OCS version

   We've added a new field to the OCS Version, which is supposed to announce the product name. The
   web ui as a client will make use of it to make the backend product and version available (e.g. for
   easier bug reports).

   https://github.com/cs3org/reva/pull/2397

*   Enhancement #2393: Update tus/tusd to version 1.8.0

   We've update tus/tusd to version 1.8.0.

   https://github.com/cs3org/reva/issues/2393
   https://github.com/cs3org/reva/pull/2224

*   Enhancement #2205: Modify group and user managers to skip fetching specified metadata

   https://github.com/cs3org/reva/pull/2205

*   Enhancement #2232: Make ocs resource info cache interoperable across drivers

   https://github.com/cs3org/reva/pull/2232

*   Enhancement #2233: Populate owner data in the ocs and ocdav services

   https://github.com/cs3org/reva/pull/2233

*   Enhancement #2278: OIDC driver changes for lightweight users

   https://github.com/cs3org/reva/pull/2278

Changelog for reva 1.17.0 (2021-12-09)
=======================================

The following sections list the changes in reva 1.17.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2305: Make sure /app/new takes `target` as absolute path
*   Fix #2303: Fix content disposition header for public links files
*   Fix #2316: Fix the share types in propfinds
*   Fix #2803: Fix app provider for editor public links
*   Fix #2298: Remove share refs from trashbin
*   Fix #2309: Remove early finish for zero byte file uploads
*   Fix #1941: Fix TUS uploads with transfer token only
*   Chg #2210: Fix app provider new file creation and improved error codes
*   Enh #2217: OIDC auth driver for ESCAPE IAM
*   Enh #2256: Return user type in the response of the ocs GET user call
*   Enh #2315: Add new attributes to public link propfinds
*   Enh #2740: Implement space membership endpoints
*   Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
*   Enh #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

Details
-------

*   Bugfix #2305: Make sure /app/new takes `target` as absolute path

   A mini-PR to make the `target` parameter absolute (by prepending `/` if missing).

   https://github.com/cs3org/reva/pull/2305

*   Bugfix #2303: Fix content disposition header for public links files

   https://github.com/cs3org/reva/pull/2303
   https://github.com/cs3org/reva/pull/2297
   https://github.com/cs3org/reva/pull/2332
   https://github.com/cs3org/reva/pull/2346

*   Bugfix #2316: Fix the share types in propfinds

   The share types for public links were not correctly added to propfinds.

   https://github.com/cs3org/reva/pull/2316

*   Bugfix #2803: Fix app provider for editor public links

   Fixed opening the app provider in public links with the editor permission. The app provider
   failed to open the file in read write mode.

   https://github.com/owncloud/ocis/issues/2803
   https://github.com/cs3org/reva/pull/2310

*   Bugfix #2298: Remove share refs from trashbin

   https://github.com/cs3org/reva/pull/2298

*   Bugfix #2309: Remove early finish for zero byte file uploads

   We've fixed the upload of zero byte files by removing the early upload finishing mechanism.

   https://github.com/cs3org/reva/issues/2309
   https://github.com/owncloud/ocis/issues/2609

*   Bugfix #1941: Fix TUS uploads with transfer token only

   TUS uploads had been stopped when the user JWT token expired, even if only the transfer token
   should be validated. Now uploads will continue as intended.

   https://github.com/cs3org/reva/pull/1941

*   Change #2210: Fix app provider new file creation and improved error codes

   We've fixed the behavior for the app provider when creating new files. Previously the app
   provider would overwrite already existing files when creating a new file, this is now handled
   and prevented. The new file endpoint accepted a path to a file, but this does not work for spaces.
   Therefore we now use the resource id of the folder where the file should be created and a filename
   to create the new file. Also the app provider returns more useful error codes in a lot of cases.

   https://github.com/cs3org/reva/pull/2210

*   Enhancement #2217: OIDC auth driver for ESCAPE IAM

   This enhancement allows for oidc token authentication via the ESCAPE IAM service.
   Authentication relies on mappings of ESCAPE IAM groups to REVA users. For a valid token, if at
   the most one group from the groups claim is mapped to one REVA user, authentication can take
   place.

   https://github.com/cs3org/reva/pull/2217

*   Enhancement #2256: Return user type in the response of the ocs GET user call

   https://github.com/cs3org/reva/pull/2256

*   Enhancement #2315: Add new attributes to public link propfinds

   Added a new property "oc:signature-auth" to public link propfinds. This is a necessary change
   to be able to support archive downloads in password protected public links.

   https://github.com/cs3org/reva/pull/2315

*   Enhancement #2740: Implement space membership endpoints

   Implemented endpoints to add and remove members to spaces.

   https://github.com/owncloud/ocis/issues/2740
   https://github.com/cs3org/reva/pull/2250

*   Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

*   Enhancement #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

   Some IDPs don't support the "preferred_username" claim. Fallback to the "email" claim in that
   case.

   https://github.com/cs3org/reva/pull/2314

Changelog for reva 1.16.0 (2021-11-19)
=======================================

The following sections list the changes in reva 1.16.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2245: Don't announce search-files capability
*   Fix #2247: Merge user ACLs from EOS to sys ACLs
*   Fix #2279: Return the inode of the version folder for files when listing in EOS
*   Fix #2294: Fix HTTP return code when path is invalid
*   Fix #2231: Fix share permission on a single file in sql share driver (cbox pkg)
*   Fix #2230: Fix open by default app and expose default app
*   Fix #2265: Fix nil pointer exception when resolving members of a group (rest driver)
*   Fix #1214: Fix restoring versions
*   Fix #2254: Fix spaces propfind
*   Fix #2260: Fix unset quota xattr on darwin
*   Fix #5776: Enforce permissions in public share apps
*   Fix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing
*   Fix #2287: Add public link access via mount-ID:token/relative-path to the scope
*   Fix #2244: Fix the permissions response for shared files in the cbox sql driver
*   Enh #2219: Add virtual view tests
*   Enh #2230: Add priority to app providers
*   Enh #2258: Improved error messages from the AppProviders
*   Enh #2119: Add authprovider owncloudsql
*   Enh #2211: Enhance the cbox share sql driver to store accepted group shares
*   Enh #2212: Filter root path according to the agent that makes the request
*   Enh #2237: Skip get user call in eosfs in case previous ones also failed
*   Enh #2266: Callback for the EOS UID cache to retry fetch for failed keys
*   Enh #2215: Aggregrate resource info properties for virtual views
*   Enh #2271: Revamp the favorite manager and add the cbox sql driver
*   Enh #2248: Cache whether a user home was created or not
*   Enh #2282: Return a proper NOT_FOUND error when a user or group is not found
*   Enh #2268: Add the reverseproxy http service
*   Enh #2207: Enable users to list all spaces
*   Enh #2286: Add trace ID to middleware loggers
*   Enh #2251: Mentix service inference
*   Enh #2218: Allow filtering of mime types supported by app providers
*   Enh #2213: Add public link share type to propfind response
*   Enh #2253: Support the file editor role for public links
*   Enh #2208: Reduce redundant stat calls when statting by resource ID
*   Enh #2235: Specify a list of allowed folders/files to be archived
*   Enh #2267: Restrict the paths where share creation is allowed
*   Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
*   Enh #2239: Update toml configs

Details
-------

*   Bugfix #2245: Don't announce search-files capability

   The `dav.reports` capability contained a `search-files` report which is currently not
   implemented. We removed it from the defaults.

   https://github.com/cs3org/reva/pull/2245

*   Bugfix #2247: Merge user ACLs from EOS to sys ACLs

   https://github.com/cs3org/reva/pull/2247

*   Bugfix #2279: Return the inode of the version folder for files when listing in EOS

   https://github.com/cs3org/reva/pull/2279

*   Bugfix #2294: Fix HTTP return code when path is invalid

   Before when a path was invalid, the archiver returned a 500 error code. Now this is fixed and
   returns a 404 code.

   https://github.com/cs3org/reva/pull/2294

*   Bugfix #2231: Fix share permission on a single file in sql share driver (cbox pkg)

   https://github.com/cs3org/reva/pull/2231

*   Bugfix #2230: Fix open by default app and expose default app

   We've fixed the open by default app name behaviour which previously only worked, if the default
   app was configured by the provider address. We also now expose the default app on the
   `/app/list` endpoint to clients.

   https://github.com/cs3org/reva/issues/2230
   https://github.com/cs3org/cs3apis/pull/157

*   Bugfix #2265: Fix nil pointer exception when resolving members of a group (rest driver)

   https://github.com/cs3org/reva/pull/2265

*   Bugfix #1214: Fix restoring versions

   Restoring a version would not remove that version from the version list. Now the behavior is
   compatible to ownCloud 10.

   https://github.com/owncloud/ocis/issues/1214
   https://github.com/cs3org/reva/pull/2270

*   Bugfix #2254: Fix spaces propfind

   Fixed the deep listing of spaces.

   https://github.com/cs3org/reva/pull/2254

*   Bugfix #2260: Fix unset quota xattr on darwin

   Unset quota attributes were creating errors in the logfile on darwin.

   https://github.com/cs3org/reva/pull/2260

*   Bugfix #5776: Enforce permissions in public share apps

   A receiver of a read-only public share could still edit files via apps like Collabora. These
   changes enforce the share permissions in apps used on publicly shared resources.

   https://github.com/owncloud/web/issues/5776
   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/22142214

*   Bugfix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing

   We've fixed the status code to 409 according to the WebDAV standard for MKCOL requests where an
   ancestor is missing. Previously these requests would fail with an different error code (eg.
   500) because of storage driver limitations (eg. oCIS FS cannot handle recursive creation of
   directories).

   https://github.com/owncloud/ocis/issues/2767
   https://github.com/cs3org/reva/pull/2293

*   Bugfix #2287: Add public link access via mount-ID:token/relative-path to the scope

   https://github.com/cs3org/reva/pull/2287

*   Bugfix #2244: Fix the permissions response for shared files in the cbox sql driver

   https://github.com/cs3org/reva/pull/2244

*   Enhancement #2219: Add virtual view tests

   https://github.com/cs3org/reva/pull/2219

*   Enhancement #2230: Add priority to app providers

   Before the order of the list returned by the method FindProviders of app providers depended
   from the order in which the app provider registered themselves. Now, it is possible to specify a
   priority for each app provider, and even if an app provider re-register itself (for example
   after a restart), the order is kept.

   https://github.com/cs3org/reva/pull/2230
   https://github.com/cs3org/cs3apis/pull/157
   https://github.com/cs3org/reva/pull/2263

*   Enhancement #2258: Improved error messages from the AppProviders

   Some rather cryptic messages are now hidden to users, and some others are made more
   user-friendly. Support for multiple locales is still missing and out of scope for now.

   https://github.com/cs3org/reva/pull/2258

*   Enhancement #2119: Add authprovider owncloudsql

   We added an authprovider that can be configured to authenticate against an owncloud classic
   mysql database. It verifies the password from the oc_users table.

   https://github.com/cs3org/reva/pull/2119

*   Enhancement #2211: Enhance the cbox share sql driver to store accepted group shares

   https://github.com/cs3org/reva/pull/2211

*   Enhancement #2212: Filter root path according to the agent that makes the request

   https://github.com/cs3org/reva/pull/2212

*   Enhancement #2237: Skip get user call in eosfs in case previous ones also failed

   https://github.com/cs3org/reva/pull/2237

*   Enhancement #2266: Callback for the EOS UID cache to retry fetch for failed keys

   https://github.com/cs3org/reva/pull/2266

*   Enhancement #2215: Aggregrate resource info properties for virtual views

   https://github.com/cs3org/reva/pull/2215

*   Enhancement #2271: Revamp the favorite manager and add the cbox sql driver

   https://github.com/cs3org/reva/pull/2271

*   Enhancement #2248: Cache whether a user home was created or not

   Previously, on every call, we used to stat the user home to make sure that it existed. Now we cache
   it for a given amount of time so as to avoid repeated calls.

   https://github.com/cs3org/reva/pull/2248

*   Enhancement #2282: Return a proper NOT_FOUND error when a user or group is not found

   https://github.com/cs3org/reva/pull/2282

*   Enhancement #2268: Add the reverseproxy http service

   This PR adds an HTTP service which does the job of authenticating incoming requests via the reva
   middleware before forwarding them to the respective backends. This is useful for extensions
   which do not have the auth mechanisms.

   https://github.com/cs3org/reva/pull/2268

*   Enhancement #2207: Enable users to list all spaces

   Added a permission check if the user has the `list-all-spaces` permission. This enables users
   to list all spaces, even those which they are not members of.

   https://github.com/cs3org/reva/pull/2207

*   Enhancement #2286: Add trace ID to middleware loggers

   https://github.com/cs3org/reva/pull/2286

*   Enhancement #2251: Mentix service inference

   Previously, 4 different services per site had to be created in the GOCDB. This PR removes this
   redundancy by infering all endpoints from a single service entity, making site
   administration a lot easier.

   https://github.com/cs3org/reva/pull/2251

*   Enhancement #2218: Allow filtering of mime types supported by app providers

   https://github.com/cs3org/reva/pull/2218

*   Enhancement #2213: Add public link share type to propfind response

   Added share type for public links to propfind responses.

   https://github.com/cs3org/reva/pull/2213
   https://github.com/cs3org/reva/pull/2257

*   Enhancement #2253: Support the file editor role for public links

   https://github.com/cs3org/reva/pull/2253

*   Enhancement #2208: Reduce redundant stat calls when statting by resource ID

   https://github.com/cs3org/reva/pull/2208

*   Enhancement #2235: Specify a list of allowed folders/files to be archived

   Adds a configuration to the archiver service in order to specify a list of folders (as regex)
   that can be archived.

   https://github.com/cs3org/reva/pull/2235

*   Enhancement #2267: Restrict the paths where share creation is allowed

   This PR limits share creation to certain specified paths. These can be useful when users have
   access to global spaces and virtual views but these should not be sharable.

   https://github.com/cs3org/reva/pull/2267

*   Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

*   Enhancement #2239: Update toml configs

   We updated the local and drone configurations, cleanad up the example configs and removed the
   reva gen subcommand which was generating outdated config.

   https://github.com/cs3org/reva/pull/2239

Changelog for reva 1.15.0 (2021-10-26)
=======================================

The following sections list the changes in reva 1.15.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2168: Override provider if was previously registered
*   Fix #2173: Fix archiver max size reached error
*   Fix #2167: Handle nil quota in decomposedfs
*   Fix #2153: Restrict EOS project spaces sharing permissions to admins and writers
*   Fix #2179: Fix the returned permissions for webdav uploads
*   Fix #2177: Retrieve the full path of a share when setting as
*   Chg #2479: Make apps able to work with public shares
*   Enh #2203: Add alerting webhook to SiteAcc service
*   Enh #2190: Update CODEOWNERS
*   Enh #2174: Inherit ACLs for files from parent directories
*   Enh #2152: Add a reference parameter to the getQuota request
*   Enh #2171: Add optional claim parameter to machine auth
*   Enh #2163: Nextcloud-based share manager for pkg/ocm/share
*   Enh #2135: Nextcloud test improvements
*   Enh #2180: Remove OCDAV options namespace parameter
*   Enh #2117: Add ocs cache warmup strategy for first request from the user
*   Enh #2170: Handle propfind requests for existing files
*   Enh #2165: Allow access to recycle bin for arbitrary paths outside homes
*   Enh #2193: Filter root paths according to user agent
*   Enh #2162: Implement the UpdateStorageSpace method
*   Enh #2189: Add user setting capability

Details
-------

*   Bugfix #2168: Override provider if was previously registered

   Previously if an AppProvider registered himself two times, for example after a failure, the
   mime types supported by the provider contained multiple times the same provider. Now this has
   been fixed, overriding the previous one.

   https://github.com/cs3org/reva/pull/2168

*   Bugfix #2173: Fix archiver max size reached error

   Previously in the total size count of the files being archived, the folders were taken into
   account, and this could cause a false max size reached error because the size of a directory is
   recursive-computed, causing the archive to be truncated. Now in the size count, the
   directories are skipped.

   https://github.com/cs3org/reva/pull/2173

*   Bugfix #2167: Handle nil quota in decomposedfs

   Do not nil pointer derefenrence when sending nil quota to decomposedfs

   https://github.com/cs3org/reva/issues/2167

*   Bugfix #2153: Restrict EOS project spaces sharing permissions to admins and writers

   https://github.com/cs3org/reva/pull/2153

*   Bugfix #2179: Fix the returned permissions for webdav uploads

   We've fixed the returned permissions for webdav uploads. It did not consider shares and public
   links for the permission calculation, but does so now.

   https://github.com/cs3org/reva/pull/2179
   https://github.com/cs3org/reva/pull/2151

*   Bugfix #2177: Retrieve the full path of a share when setting as

   Accepted or on shared by me

   https://github.com/cs3org/reva/pull/2177

*   Change #2479: Make apps able to work with public shares

   Public share receivers were not possible to use apps in public shares because the apps couldn't
   load the files in the public shares. This has now been made possible by changing the scope checks
   for public shares.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2143

*   Enhancement #2203: Add alerting webhook to SiteAcc service

   To integrate email alerting with the monitoring pipeline, a Prometheus webhook has been added
   to the SiteAcc service. Furthermore account settings have been extended/modified
   accordingly.

   https://github.com/cs3org/reva/pull/2203

*   Enhancement #2190: Update CODEOWNERS

   https://github.com/cs3org/reva/pull/2190

*   Enhancement #2174: Inherit ACLs for files from parent directories

   https://github.com/cs3org/reva/pull/2174

*   Enhancement #2152: Add a reference parameter to the getQuota request

   Implementation of [cs3org/cs3apis#147](https://github.com/cs3org/cs3apis/pull/147)

   Make the cs3apis accept a Reference in the getQuota Request to limit the call to a specific
   storage space.

   https://github.com/cs3org/reva/pull/2152
   https://github.com/cs3org/reva/pull/2178
   https://github.com/cs3org/reva/pull/2187

*   Enhancement #2171: Add optional claim parameter to machine auth

   https://github.com/cs3org/reva/issues/2171
   https://github.com/cs3org/reva/pull/2176

*   Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

*   Enhancement #2135: Nextcloud test improvements

   https://github.com/cs3org/reva/pull/2135

*   Enhancement #2180: Remove OCDAV options namespace parameter

   We dropped the namespace parameter, as it is not used in the options handler.

   https://github.com/cs3org/reva/pull/2180

*   Enhancement #2117: Add ocs cache warmup strategy for first request from the user

   https://github.com/cs3org/reva/pull/2117

*   Enhancement #2170: Handle propfind requests for existing files

   https://github.com/cs3org/reva/pull/2170

*   Enhancement #2165: Allow access to recycle bin for arbitrary paths outside homes

   https://github.com/cs3org/reva/pull/2165
   https://github.com/cs3org/reva/pull/2188

*   Enhancement #2193: Filter root paths according to user agent

   Adds a new rule setting in the storage registry ("allowed_user_agents"), that allows a user to
   specify which storage provider shows according to the user agent that made the request.

   https://github.com/cs3org/reva/pull/2193

*   Enhancement #2162: Implement the UpdateStorageSpace method

   Added the UpdateStorageSpace method to the decomposedfs.

   https://github.com/cs3org/reva/pull/2162
   https://github.com/cs3org/reva/pull/2195
   https://github.com/cs3org/reva/pull/2196

*   Enhancement #2189: Add user setting capability

   We've added a capability to communicate the existance of a user settings service to clients.

   https://github.com/owncloud/web/issues/5926
   https://github.com/cs3org/reva/pull/2189
   https://github.com/owncloud/ocis/pull/2655

Changelog for reva 1.14.0 (2021-10-12)
=======================================

The following sections list the changes in reva 1.14.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2103: AppProvider: propagate back errors reported by WOPI
*   Fix #2149: Remove excess info from the http list app providers endpoint
*   Fix #2114: Add as default app while registering and skip unset mimetypes
*   Fix #2095: Fix app open when multiple app providers are present
*   Fix #2135: Make TUS capabilities configurable
*   Fix #2076: Fix chi routing
*   Fix #2077: Fix concurrent registration of mimetypes
*   Fix #2154: Return OK when trying to delete a non existing reference
*   Fix #2078: Fix nil pointer exception in stat
*   Fix #2073: Fix opening a readonly filetype with WOPI
*   Fix #2140: Map GRPC error codes to REVA errors
*   Fix #2147: Follow up of #2138: this is the new expected format
*   Fix #2116: Differentiate share types when retrieving received shares in sql driver
*   Fix #2074: Fix Stat() for EOS storage provider
*   Fix #2151: Fix return code for webdav uploads when the token expired
*   Chg #2121: Sharemanager API change
*   Enh #2090: Return space name during list storage spaces
*   Enh #2138: Default AppProvider on top of the providers list
*   Enh #2137: Revamp app registry and add parameter to control file creation
*   Enh #145: UI improvements for the AppProviders
*   Enh #2088: Add archiver and app provider to ocs capabilities
*   Enh #2537: Add maximum files and size to archiver capabilities
*   Enh #2100: Add support for resource id to the archiver
*   Enh #2158: Augment the Id of new spaces
*   Enh #2085: Make encoding user groups in access tokens configurable
*   Enh #146: Filter the denial shares (permission = 0) out of
*   Enh #2141: Use golang v1.17
*   Enh #2053: Safer defaults for TLS verification on LDAP connections
*   Enh #2115: Reduce code duplication in LDAP related drivers
*   Enh #1989: Add redirects from OC10 URL formats
*   Enh #2479: Limit publicshare and resourceinfo scope content
*   Enh #2071: Implement listing favorites via the dav report API
*   Enh #2091: Nextcloud share managers
*   Enh #2070: More unit tests for the Nextcloud storage provider
*   Enh #2087: More unit tests for the Nextcloud auth and user managers
*   Enh #2075: Make owncloudsql leverage existing filecache index
*   Enh #2050: Add a share types filter to the OCS API
*   Enh #2134: Use space Type from request
*   Enh #2132: Align local tests with drone setup
*   Enh #2095: Whitelisting for apps
*   Enh #2155: Pass an extra query parameter to WOPI /openinapp with a

Details
-------

*   Bugfix #2103: AppProvider: propagate back errors reported by WOPI

   On /app/open and return base64-encoded fileids on /app/new

   https://github.com/cs3org/reva/pull/2103

*   Bugfix #2149: Remove excess info from the http list app providers endpoint

   We've removed excess info from the http list app providers endpoint. The app provider section
   contained all mime types supported by a certain app provider, which led to a very big JSON
   payload and since they are not used they have been removed again. Mime types not on the mime type
   configuration list always had `application/octet-stream` as a file extension and
   `APPLICATION/OCTET-STREAM file` as name and description. Now these information are just
   omitted.

   https://github.com/cs3org/reva/pull/2149
   https://github.com/owncloud/ocis/pull/2603
   https://github.com/cs3org/reva/pull/2138

*   Bugfix #2114: Add as default app while registering and skip unset mimetypes

   We fixed that app providers will be set as default app while registering if configured. Also we
   changed the behaviour that listing supported mimetypes only displays allowed / configured
   mimetypes.

   https://github.com/cs3org/reva/pull/2114
   https://github.com/cs3org/reva/pull/2095

*   Bugfix #2095: Fix app open when multiple app providers are present

   We've fixed the gateway behavior, that when multiple app providers are present, it always
   returned that we have duplicate names for app providers. This was due the call to
   GetAllProviders() without any subsequent filtering by name. Now this filter mechanism is in
   place and the duplicate app providers error will only appear if a real duplicate is found.

   https://github.com/cs3org/reva/issues/2095
   https://github.com/cs3org/reva/pull/2117

*   Bugfix #2135: Make TUS capabilities configurable

   We've fixed the configuration for the TUS capabilities, which will now take the given
   configuration instead of always using hardcoded defaults.

   https://github.com/cs3org/reva/pull/2135

*   Bugfix #2076: Fix chi routing

   Chi routes based on the URL.RawPath, which is not updated by the shiftPath based routing used in
   reva. By setting the RawPath to an empty string chi will fall pack to URL.Path, allowing it to
   match percent encoded path segments, e.g. when trying to match emails or multibyte
   characters.

   https://github.com/cs3org/reva/pull/2076

*   Bugfix #2077: Fix concurrent registration of mimetypes

   We fixed registering mimetypes in the mime package when starting multiple storage providers
   in the same process.

   https://github.com/cs3org/reva/pull/2077

*   Bugfix #2154: Return OK when trying to delete a non existing reference

   When the gateway declines a share we can ignore a non existing reference.

   https://github.com/cs3org/reva/pull/2154
   https://github.com/owncloud/ocis/pull/2603

*   Bugfix #2078: Fix nil pointer exception in stat

   https://github.com/cs3org/reva/pull/2078

*   Bugfix #2073: Fix opening a readonly filetype with WOPI

   This change fixes the opening of filetypes that are only supported to be viewed and not to be
   edited by some WOPI compliant office suites.

   https://github.com/cs3org/reva/pull/2073

*   Bugfix #2140: Map GRPC error codes to REVA errors

   We've fixed the error return behaviour in the gateway which would return GRPC error codes from
   the auth middleware. Now it returns REVA errors which other parts of REVA are also able to
   understand.

   https://github.com/cs3org/reva/pull/2140

*   Bugfix #2147: Follow up of #2138: this is the new expected format

   For the mime types configuration for the AppRegistry.

   https://github.com/cs3org/reva/pull/2147

*   Bugfix #2116: Differentiate share types when retrieving received shares in sql driver

   https://github.com/cs3org/reva/pull/2116

*   Bugfix #2074: Fix Stat() for EOS storage provider

   This change fixes the convertion between the eosclient.FileInfo to ResourceInfo, in which
   the field ArbitraryMetadata was missing. Moreover, to be consistent with
   SetArbitraryMetadata() EOS implementation, all the "user." prefix are stripped out from the
   xattrs.

   https://github.com/cs3org/reva/pull/2074

*   Bugfix #2151: Fix return code for webdav uploads when the token expired

   We've fixed the behavior webdav uploads when the token expired before the final stat.
   Previously clients would receive a http 500 error which is wrong, because the file was
   successfully uploaded and only the stat couldn't be performed. Now we return a http 200 ok and
   the clients will fetch the file info in a separate propfind request.

   Also we introduced the upload expires header on the webdav/TUS and datagateway endpoints, to
   signal clients how long an upload can be performed.

   https://github.com/cs3org/reva/pull/2151

*   Change #2121: Sharemanager API change

   This PR updates reva to reflect the share manager CS3 API changes.

   https://github.com/cs3org/reva/pull/2121

*   Enhancement #2090: Return space name during list storage spaces

   In the decomposedfs we return now the space name in the response which is stored in the extended
   attributes.

   https://github.com/cs3org/reva/issues/2090

*   Enhancement #2138: Default AppProvider on top of the providers list

   For each mime type

   Now for each mime type, when asking for the list of mime types, the default AppProvider, set both
   using the config and the SetDefaultProviderForMimeType method, is always in the top of the
   list of AppProviders. The config for the Providers and Mime Types for the AppRegistry changed,
   using a list instead of a map. In fact the list of mime types returned by ListSupportedMimeTypes
   is now ordered according the config.

   https://github.com/cs3org/reva/pull/2138

*   Enhancement #2137: Revamp app registry and add parameter to control file creation

   https://github.com/cs3org/reva/pull/2137

*   Enhancement #145: UI improvements for the AppProviders

   Mime types and their friendly names are now handled in the /app/list HTTP endpoint, and an
   additional /app/new endpoint is made available to create new files for apps.

   https://github.com/cs3org/cs3apis/pull/145
   https://github.com/cs3org/reva/pull/2067

*   Enhancement #2088: Add archiver and app provider to ocs capabilities

   The archiver and app provider has been added to the ocs capabilities.

   https://github.com/cs3org/reva/pull/2088
   https://github.com/owncloud/ocis/pull/2529

*   Enhancement #2537: Add maximum files and size to archiver capabilities

   We added the maximum files count and maximum archive size of the archiver to the capabilities
   endpoint. Clients can use this to generate warnings before the actual archive creation fails.

   https://github.com/owncloud/ocis/issues/2537
   https://github.com/cs3org/reva/pull/2105

*   Enhancement #2100: Add support for resource id to the archiver

   Before the archiver only supported resources provided by a path. Now also the resources ID are
   supported in order to specify the content of the archive to download. The parameters accepted
   by the archiver are two: an optional list of `path` (containing the paths of the resources) and
   an optional list of `id` (containing the resources IDs of the resources).

   https://github.com/cs3org/reva/issues/2097
   https://github.com/cs3org/reva/pull/2100

*   Enhancement #2158: Augment the Id of new spaces

   Newly created spaces were missing the Root reference and the storage id in the space id.

   https://github.com/cs3org/reva/issues/2158

*   Enhancement #2085: Make encoding user groups in access tokens configurable

   https://github.com/cs3org/reva/pull/2085

*   Enhancement #146: Filter the denial shares (permission = 0) out of

   The Shared-with-me UI view. Also they work regardless whether they are accepted or not,
   therefore there's no point to expose them.

   https://github.com/cs3org/cs3apis/pull/146
   https://github.com/cs3org/reva/pull/2072

*   Enhancement #2141: Use golang v1.17

   https://github.com/cs3org/reva/pull/2141

*   Enhancement #2053: Safer defaults for TLS verification on LDAP connections

   The LDAP client connections were hardcoded to ignore certificate validation errors. Now
   verification is enabled by default and a new config parameter 'insecure' is introduced to
   override that default. It is also possible to add trusted Certificates by using the new
   'cacert' config paramter.

   https://github.com/cs3org/reva/pull/2053

*   Enhancement #2115: Reduce code duplication in LDAP related drivers

   https://github.com/cs3org/reva/pull/2115

*   Enhancement #1989: Add redirects from OC10 URL formats

   Added redirectors for ownCloud 10 URLs. This allows users to continue to use their bookmarks
   from ownCloud 10 in ocis.

   https://github.com/cs3org/reva/pull/1989

*   Enhancement #2479: Limit publicshare and resourceinfo scope content

   We changed the publicshare and resourceinfo scopes to contain only necessary values. This
   reduces the size of the resulting token and also limits the amount of data which can be leaked.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2093

*   Enhancement #2071: Implement listing favorites via the dav report API

   Added filter-files to the dav REPORT API. This enables the listing of favorites.

   https://github.com/cs3org/reva/pull/2071
   https://github.com/cs3org/reva/pull/2086

*   Enhancement #2091: Nextcloud share managers

   Share manager that uses Nextcloud as a backend

   https://github.com/cs3org/reva/pull/2091

*   Enhancement #2070: More unit tests for the Nextcloud storage provider

   Adds more unit tests for the Nextcloud storage provider.

   https://github.com/cs3org/reva/pull/2070

*   Enhancement #2087: More unit tests for the Nextcloud auth and user managers

   Adds more unit tests for the Nextcloud auth manager and the Nextcloud user manager

   https://github.com/cs3org/reva/pull/2087

*   Enhancement #2075: Make owncloudsql leverage existing filecache index

   When listing folders the SQL query now uses an existing index on the filecache table.

   https://github.com/cs3org/reva/pull/2075

*   Enhancement #2050: Add a share types filter to the OCS API

   Added a filter to the OCS API to filter the received shares by type.

   https://github.com/cs3org/reva/pull/2050

*   Enhancement #2134: Use space Type from request

   In the decomposedfs we now use the space type from the request when creating a new space.

   https://github.com/cs3org/reva/issues/2134

*   Enhancement #2132: Align local tests with drone setup

   We fixed running the tests locally and align it with the drone setup.

   https://github.com/cs3org/reva/issues/2132

*   Enhancement #2095: Whitelisting for apps

   AppProvider supported mime types are now overridden in its configuration. A friendly name, a
   description, an extension, an icon and a default app, can be configured in the AppRegistry for
   each mime type.

   https://github.com/cs3org/reva/pull/2095

*   Enhancement #2155: Pass an extra query parameter to WOPI /openinapp with a

   Unique and consistent over time user identifier. The Reva token used so far is not consistent
   (it's per session) and also too long.

   https://github.com/cs3org/reva/pull/2155
   https://github.com/cs3org/wopiserver/pull/48

Changelog for reva 1.13.0 (2021-09-14)
=======================================

The following sections list the changes in reva 1.13.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #2024: Fixes for http appprovider endpoints
*   Fix #2054: Fix the response after deleting a share
*   Fix #2026: Fix moving of a shared file
*   Fix #2047: Do not truncate logs on restart
*   Fix #1605: Allow to expose full paths in OCS API
*   Fix #2033: Fix the storage id of shares
*   Fix #2059: Remove "Got registration for user manager" print statements
*   Fix #2051: Remove malformed parameters from WOPI discovery URLs
*   Fix #2055: Fix uploads of empty files
*   Fix #1991: Remove share references when declining shares
*   Fix #2030: Fix superfluous WriteHeader on file upload
*   Enh #2034: Fail initialization of a WOPI AppProvider if
*   Enh #1968: Use a URL object in OpenInAppResponse
*   Enh #1698: Implement folder download as archive
*   Enh #2042: Escape ldap filters
*   Enh #2028: Machine auth provider
*   Enh #2043: Nextcloud user backend
*   Enh #2006: Move ocs API to go-chi/chi based URL routing
*   Enh #1994: Add owncloudsql driver for the userprovider
*   Enh #1971: Add documentation for runtime-plugins
*   Enh #2044: Add utility methods for creating share filters
*   Enh #2065: New sharing role Manager
*   Enh #2015: Add spaces to the list of capabilities
*   Enh #2041: Create operations for Spaces
*   Enh #2029: Tracing agent configuration

Details
-------

*   Bugfix #2024: Fixes for http appprovider endpoints

   https://github.com/cs3org/reva/pull/2024
   https://github.com/cs3org/reva/pull/1968

*   Bugfix #2054: Fix the response after deleting a share

   Added the deleted share to the response after deleting it.

   https://github.com/cs3org/reva/pull/2054

*   Bugfix #2026: Fix moving of a shared file

   As the share receiver, moving a shared file to another share was not possible.

   https://github.com/cs3org/reva/pull/2026

*   Bugfix #2047: Do not truncate logs on restart

   This change fixes the way log files were opened. Before they were truncated and now the log file
   will be open in append mode and created it if it does not exist.

   https://github.com/cs3org/reva/pull/2047

*   Bugfix #1605: Allow to expose full paths in OCS API

   Before this fix a share file_target was always harcoded to use a base path. This fix provides the
   possiblity to expose full paths in the OCIS API and asymptotically in OCIS web.

   https://github.com/cs3org/reva/pull/1605

*   Bugfix #2033: Fix the storage id of shares

   The storageid in the share object contained an incorrect value.

   https://github.com/cs3org/reva/pull/2033

*   Bugfix #2059: Remove "Got registration for user manager" print statements

   Removed the "Got registration for user manager" print statements which spams the log output
   without respecting any log level.

   https://github.com/cs3org/reva/pull/2059

*   Bugfix #2051: Remove malformed parameters from WOPI discovery URLs

   This change fixes the parsing of WOPI discovery URLs for MSOffice /hosting/discovery
   endpoint. This endpoint is known to contain malformed query paramters and therefore this fix
   removes them.

   https://github.com/cs3org/reva/pull/2051

*   Bugfix #2055: Fix uploads of empty files

   This change fixes upload of empty files. Previously this was broken and only worked for the
   owncloud filesystem as it bypasses the semantics of the InitiateFileUpload call to touch a
   local file.

   https://github.com/cs3org/reva/pull/2055

*   Bugfix #1991: Remove share references when declining shares

   Implemented the removal of share references when a share gets declined. Now when a user
   declines a share it will no longer be listed in their `Shares` directory.

   https://github.com/cs3org/reva/pull/1991

*   Bugfix #2030: Fix superfluous WriteHeader on file upload

   Removes superfluous Writeheader on file upload and therefore removes the error message
   "http: superfluous response.WriteHeader call from
   github.com/cs3org/reva/internal/http/interceptors/log.(*responseLogger).WriteHeader
   (log.go:154)"

   https://github.com/cs3org/reva/pull/2030

*   Enhancement #2034: Fail initialization of a WOPI AppProvider if

   The underlying app is not WOPI-compliant nor it is supported by the WOPI bridge extensions

   https://github.com/cs3org/reva/pull/2034

*   Enhancement #1968: Use a URL object in OpenInAppResponse

   https://github.com/cs3org/reva/pull/1968

*   Enhancement #1698: Implement folder download as archive

   Adds a new http service which will create an archive (platform dependent, zip in windows and tar
   in linux) given a list of file.

   https://github.com/cs3org/reva/issues/1698
   https://github.com/cs3org/reva/pull/2066

*   Enhancement #2042: Escape ldap filters

   Added ldap filter escaping to increase the security of reva.

   https://github.com/cs3org/reva/pull/2042

*   Enhancement #2028: Machine auth provider

   Adds a new authentication method used to impersonate users, using a shared secret, called
   api-key.

   https://github.com/cs3org/reva/pull/2028

*   Enhancement #2043: Nextcloud user backend

   Adds Nextcloud as a user backend (Nextcloud drivers for 'auth' and 'user'). Also adds back the
   Nextcloud storage integration tests.

   https://github.com/cs3org/reva/pull/2043

*   Enhancement #2006: Move ocs API to go-chi/chi based URL routing

   https://github.com/cs3org/reva/issues/1986
   https://github.com/cs3org/reva/pull/2006

*   Enhancement #1994: Add owncloudsql driver for the userprovider

   We added a new backend for the userprovider that is backed by an owncloud 10 database. By default
   the `user_id` column is used as the reva user username and reva user opaque id. When setting
   `join_username=true` the reva user username is joined from the `oc_preferences` table
   (`appid='core' AND configkey='username'`) instead. When setting
   `join_ownclouduuid=true` the reva user opaqueid is joined from the `oc_preferences` table
   (`appid='core' AND configkey='ownclouduuid'`) instead. This allows more flexible
   migration strategies. It also supports a `enable_medial_search` config option when
   searching users that will enclose the query with `%`.

   https://github.com/cs3org/reva/pull/1994

*   Enhancement #1971: Add documentation for runtime-plugins

   https://github.com/cs3org/reva/pull/1971

*   Enhancement #2044: Add utility methods for creating share filters

   Updated the CS3 API to include the new share grantee filter and added utility methods for
   creating share filters. This will help making the code more concise.

   https://github.com/cs3org/reva/pull/2044

*   Enhancement #2065: New sharing role Manager

   The new Manager role is equivalent to a Co-Owner with the difference that a Manager can create
   grants on the root of the Space. This means inviting a user to a space will not require an action
   from them, as the Manager assigns the grants.

   https://github.com/cs3org/reva/pull/2065

*   Enhancement #2015: Add spaces to the list of capabilities

   In order for clients to be aware of the new spaces feature we need to enable the `spaces` flag on
   the capabilities' endpoint.

   https://github.com/cs3org/reva/pull/2015

*   Enhancement #2041: Create operations for Spaces

   DecomposedFS is aware now of the concept of Spaces, and supports for creating them.

   https://github.com/cs3org/reva/pull/2041

*   Enhancement #2029: Tracing agent configuration

   Earlier we could only use the collector URL directly, but since an agent can be deployed as a
   sidecar process it makes much more sense to use it instead of the collector directly.

   https://github.com/cs3org/reva/pull/2029

Changelog for reva 1.12.0 (2021-08-24)
=======================================

The following sections list the changes in reva 1.12.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1819: Disable notifications
*   Fix #2000: Fix dependency on tests
*   Fix #1957: Fix etag propagation on deletes
*   Fix #1960: Return the updated share after updating
*   Fix #1993: Fix owncloudsql GetMD
*   Fix #1954: Fix response format of the sharees API
*   Fix #1965: Fix the file target of user and group shares
*   Fix #1956: Fix trashbin listing with depth 0
*   Fix #1987: Fix windows build
*   Fix #1990: Increase oc10 compatibility of owncloudsql
*   Fix #1978: Owner type is optional
*   Fix #1980: Propagate the etag after restoring a file version
*   Fix #1985: Add quota stubs
*   Fix #1992: Check if symlink exists instead of spamming the console
*   Fix #1913: Logic to restore files to readonly nodes
*   Chg #1982: Move user context methods into a separate `userctx` package
*   Enh #1946: Add share manager that connects to oc10 databases
*   Enh #1983: Add Codacy unit test coverage
*   Enh #1803: Introduce new webdav spaces endpoint
*   Enh #1998: Initial version of the Nextcloud storage driver
*   Enh #1984: Replace OpenCensus with OpenTelemetry
*   Enh #1861: Add support for runtime plugins
*   Enh #2008: Site account extensions

Details
-------

*   Bugfix #1819: Disable notifications

   The presence of the key `notifications` in the capabilities' response would cause clients to
   attempt to poll the notifications endpoint, which is not yet supported. To prevent the
   unnecessary bandwidth we are disabling this altogether.

   https://github.com/cs3org/reva/pull/1819

*   Bugfix #2000: Fix dependency on tests

   The Nextcloud storage driver depended on a mock http client from the tests/ folder This broke
   the Docker build The dependency was removed A check was added to test the Docker build on each PR

   https://github.com/cs3org/reva/pull/2000

*   Bugfix #1957: Fix etag propagation on deletes

   When deleting a file the etag propagation would skip the parent of the deleted file.

   https://github.com/cs3org/reva/pull/1957

*   Bugfix #1960: Return the updated share after updating

   When updating the state of a share in the in-memory share manager the old share state was
   returned instead of the updated state.

   https://github.com/cs3org/reva/pull/1960

*   Bugfix #1993: Fix owncloudsql GetMD

   The GetMD call internally was not prefixing the path when looking up resources by id.

   https://github.com/cs3org/reva/pull/1993

*   Bugfix #1954: Fix response format of the sharees API

   The sharees API wasn't returning the users and groups arrays correctly.

   https://github.com/cs3org/reva/pull/1954

*   Bugfix #1965: Fix the file target of user and group shares

   In some cases the file target of user and group shares was not properly prefixed.

   https://github.com/cs3org/reva/pull/1965
   https://github.com/cs3org/reva/pull/1967

*   Bugfix #1956: Fix trashbin listing with depth 0

   The trashbin API handled requests with depth 0 the same as request with a depth of 1.

   https://github.com/cs3org/reva/pull/1956

*   Bugfix #1987: Fix windows build

   Add the necessary `golang.org/x/sys/windows` package import to `owncloud` and
   `owncloudsql` storage drivers.

   https://github.com/cs3org/reva/pull/1987

*   Bugfix #1990: Increase oc10 compatibility of owncloudsql

   We added a few changes to the owncloudsql storage driver to behave more like oc10.

   https://github.com/cs3org/reva/pull/1990

*   Bugfix #1978: Owner type is optional

   When reading the user from the extended attributes the user type might not be set, in this case we
   now return a user with an invalid type, which correctly reflects the state on disk.

   https://github.com/cs3org/reva/pull/1978

*   Bugfix #1980: Propagate the etag after restoring a file version

   The decomposedfs didn't propagate after restoring a file version.

   https://github.com/cs3org/reva/pull/1980

*   Bugfix #1985: Add quota stubs

   The `owncloud` and `owncloudsql` drivers now read the available quota from disk to no longer
   always return 0, which causes the web UI to disable uploads.

   https://github.com/cs3org/reva/pull/1985

*   Bugfix #1992: Check if symlink exists instead of spamming the console

   The logs have been spammed with messages like `could not create symlink for ...` when using the
   decomposedfs, eg. with the oCIS storage. We now check if the link exists before trying to create
   it.

   https://github.com/cs3org/reva/pull/1992

*   Bugfix #1913: Logic to restore files to readonly nodes

   This impacts solely the DecomposedFS. Prior to these changes there was no validation when a
   user tried to restore a file from the trashbin to a share location (i.e any folder under
   `/Shares`).

   With this patch if the user restoring the resource has write permissions on the share, restore
   is possible.

   https://github.com/cs3org/reva/pull/1913

*   Change #1982: Move user context methods into a separate `userctx` package

   https://github.com/cs3org/reva/pull/1982

*   Enhancement #1946: Add share manager that connects to oc10 databases

   https://github.com/cs3org/reva/pull/1946

*   Enhancement #1983: Add Codacy unit test coverage

   This PR adds unit test coverage upload to Codacy.

   https://github.com/cs3org/reva/pull/1983

*   Enhancement #1803: Introduce new webdav spaces endpoint

   Clients can now use a new webdav endpoint
   `/dav/spaces/<storagespaceid>/relative/path/to/file` to directly access storage
   spaces.

   The `<storagespaceid>` can be retrieved using the ListStorageSpaces CS3 api call.

   https://github.com/cs3org/reva/pull/1803

*   Enhancement #1998: Initial version of the Nextcloud storage driver

   This is not usable yet in isolation, but it's a first component of
   https://github.com/pondersource/sciencemesh-nextcloud

   https://github.com/cs3org/reva/pull/1998

*   Enhancement #1984: Replace OpenCensus with OpenTelemetry

   OpenTelemetry](https://opentelemetry.io/docs/concepts/what-is-opentelemetry/) is
   an [open standard](https://github.com/open-telemetry/opentelemetry-specification) a
   sandbox CNCF project and it was formed through a merger of the OpenTracing and OpenCensus.

   > OpenCensus and OpenTracing have merged to form OpenTelemetry, which serves as the next major
   version of OpenCensus and OpenTracing. OpenTelemetry will offer backwards compatibility
   with existing OpenCensus integrations, and we will continue to make security patches to
   existing OpenCensus libraries for two years.

   There is a lot of outdated documentation as a result of this merger, and we will be better off
   adopting the latest standard and libraries.

   https://github.com/cs3org/reva/pull/1984

*   Enhancement #1861: Add support for runtime plugins

   This PR introduces a new plugin package, that allows loading external plugins into Reva at
   runtime. The hashicorp go-plugin framework was used to facilitate the plugin loading and
   communication.

   https://github.com/cs3org/reva/pull/1861

*   Enhancement #2008: Site account extensions

   This PR heavily extends the site accounts service: * Extended the accounts information (not
   just email and name) * Accounts now have a password * Users can now "log in" to their accounts and
   edit it * Ability to grant access to the GOCDB

   Furthermore, these accounts can now be used to authenticate for logging in to our customized
   GOCDB. More use cases for these accounts are also planned.

   https://github.com/cs3org/reva/pull/2008

Changelog for reva 1.11.0 (2021-08-03)
=======================================

The following sections list the changes in reva 1.11.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1899: Fix chunked uploads for new versions
*   Fix #1906: Fix copy over existing resource
*   Fix #1891: Delete Shared Resources as Receiver
*   Fix #1907: Error when creating folder with existing name
*   Fix #1937: Do not overwrite more specific matches when finding storage providers
*   Fix #1939: Fix the share jail permissions in the decomposedfs
*   Fix #1932: Numerous fixes to the owncloudsql storage driver
*   Fix #1912: Fix response when listing versions of another user
*   Fix #1910: Get user groups recursively in the cbox rest user driver
*   Fix #1904: Set Content-Length to 0 when swallowing body in the datagateway
*   Fix #1911: Fix version order in propfind responses
*   Fix #1926: Trash Bin in oCIS Storage Operations
*   Fix #1901: Fix response code when folder doesnt exist on upload
*   Enh #1785: Extend app registry with AddProvider method and mimetype filters
*   Enh #1938: Add methods to get and put context values
*   Enh #1798: Add support for a deny-all permission on references
*   Enh #1916: Generate updated protobuf bindings for EOS GRPC
*   Enh #1887: Add "a" and "l" filter for grappa queries
*   Enh #1919: Run gofmt before building
*   Enh #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)
*   Enh #1944: Implement listing supported mime types in app registry
*   Enh #1870: Be defensive about wrongly quoted etags
*   Enh #1940: Reduce memory usage when uploading with S3ng storage
*   Enh #1888: Refactoring of the webdav code
*   Enh #1900: Check for illegal names while uploading or moving files
*   Enh #1925: Refactor listing and statting across providers for virtual views

Details
-------

*   Bugfix #1899: Fix chunked uploads for new versions

   Chunked uploads didn't create a new version, when the file to upload already existed.

   https://github.com/cs3org/reva/pull/1899

*   Bugfix #1906: Fix copy over existing resource

   When the target of a copy already exists, the existing resource will be moved to the trashbin
   before executing the copy.

   https://github.com/cs3org/reva/pull/1906

*   Bugfix #1891: Delete Shared Resources as Receiver

   It is now possible to delete a shared resource as a receiver and not having the data ending up in
   the receiver's trash bin, causing a possible leak.

   https://github.com/cs3org/reva/pull/1891

*   Bugfix #1907: Error when creating folder with existing name

   When a user tried to create a folder with the name of an existing file or folder the service didn't
   return a response body containing the error.

   https://github.com/cs3org/reva/pull/1907

*   Bugfix #1937: Do not overwrite more specific matches when finding storage providers

   Depending on the order of rules in the registry it could happend that more specific matches
   (e.g. /home/Shares) were overwritten by more general ones (e.g. /home). This PR makes sure
   that the registry always returns the most specific match.

   https://github.com/cs3org/reva/pull/1937

*   Bugfix #1939: Fix the share jail permissions in the decomposedfs

   The share jail should be not writable

   https://github.com/cs3org/reva/pull/1939

*   Bugfix #1932: Numerous fixes to the owncloudsql storage driver

   The owncloudsql storage driver received numerous bugfixes and cleanups.

   https://github.com/cs3org/reva/pull/1932

*   Bugfix #1912: Fix response when listing versions of another user

   The OCS API returned the wrong response when a user tried to list the versions of another user's
   file.

   https://github.com/cs3org/reva/pull/1912

*   Bugfix #1910: Get user groups recursively in the cbox rest user driver

   https://github.com/cs3org/reva/pull/1910

*   Bugfix #1904: Set Content-Length to 0 when swallowing body in the datagateway

   When swallowing the body the Content-Lenght needs to be set to 0 to prevent proxies from reading
   the body.

   https://github.com/cs3org/reva/pull/1904

*   Bugfix #1911: Fix version order in propfind responses

   The order of the file versions in propfind responses was incorrect.

   https://github.com/cs3org/reva/pull/1911

*   Bugfix #1926: Trash Bin in oCIS Storage Operations

   Support for restoring a target folder nested deep inside the trash bin in oCIS storage. The use
   case is:

   ```console curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X
   MOVE -H 'Destination:
   https://localhost:9200/remote.php/dav/files/einstein/destination' ```

   The previous command creates the `destination` folder and moves the contents of
   `/trash-bin/einstein/f1/f2` onto it.

   Retro-compatibility in the response code with ownCloud 10. Restoring a collection to a
   non-existent nested target is not supported and MUST return `409`. The use case is:

   ```console curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X
   MOVE -H 'Destination:
   https://localhost:9200/remote.php/dav/files/einstein/this/does/not/exist' ```

   The previous command used to return `404` instead of the expected `409` by the clients.

   https://github.com/cs3org/reva/pull/1926

*   Bugfix #1901: Fix response code when folder doesnt exist on upload

   When a new file was uploaded to a non existent folder the response code was incorrect.

   https://github.com/cs3org/reva/pull/1901

*   Enhancement #1785: Extend app registry with AddProvider method and mimetype filters

   https://github.com/cs3org/reva/issues/1779
   https://github.com/cs3org/reva/pull/1785
   https://github.com/cs3org/cs3apis/pull/131

*   Enhancement #1938: Add methods to get and put context values

   Added `GetKeyValues` and `PutKeyValues` methods to fetch/put values from/to context.

   https://github.com/cs3org/reva/pull/1938

*   Enhancement #1798: Add support for a deny-all permission on references

   And implement it on the EOS storage

   http://github.com/cs3org/reva/pull/1798

*   Enhancement #1916: Generate updated protobuf bindings for EOS GRPC

   https://github.com/cs3org/reva/pull/1916

*   Enhancement #1887: Add "a" and "l" filter for grappa queries

   This PR adds the namespace filters "a" and "l" for grappa queries. With no filter will look into
   primary and e-groups, with "a" will look into primary/secondary/service/e-groups and with
   "l" will look into lightweight accounts.

   https://github.com/cs3org/reva/issues/1773
   https://github.com/cs3org/reva/pull/1887

*   Enhancement #1919: Run gofmt before building

   https://github.com/cs3org/reva/pull/1919

*   Enhancement #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)

   https://github.com/cs3org/reva/pull/1927

*   Enhancement #1944: Implement listing supported mime types in app registry

   https://github.com/cs3org/reva/pull/1944

*   Enhancement #1870: Be defensive about wrongly quoted etags

   When ocdav renders etags it will now try to correct them to the definition as *quoted strings*
   which do not contain `"`. This prevents double or triple quoted etags on the webdav api.

   https://github.com/cs3org/reva/pull/1870

*   Enhancement #1940: Reduce memory usage when uploading with S3ng storage

   The memory usage could be high when uploading files using the S3ng storage. By providing the
   actual file size when triggering `PutObject`, the overall memory usage is reduced.

   https://github.com/cs3org/reva/pull/1940

*   Enhancement #1888: Refactoring of the webdav code

   Refactored the webdav code to make it reusable.

   https://github.com/cs3org/reva/pull/1888

*   Enhancement #1900: Check for illegal names while uploading or moving files

   The code was not checking for invalid file names during uploads and moves.

   https://github.com/cs3org/reva/pull/1900

*   Enhancement #1925: Refactor listing and statting across providers for virtual views

   https://github.com/cs3org/reva/pull/1925

Changelog for reva 1.10.0 (2021-07-13)
=======================================

The following sections list the changes in reva 1.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1883: Pass directories with trailing slashes to eosclient.GenerateToken
*   Fix #1878: Improve the webdav error handling in the trashbin
*   Fix #1884: Do not send body on failed range request
*   Enh #1744: Add support for lightweight user types

Details
-------

*   Bugfix #1883: Pass directories with trailing slashes to eosclient.GenerateToken

   https://github.com/cs3org/reva/pull/1883

*   Bugfix #1878: Improve the webdav error handling in the trashbin

   The trashbin handles errors better now on the webdav endpoint.

   https://github.com/cs3org/reva/pull/1878

*   Bugfix #1884: Do not send body on failed range request

   Instead of send the error in the body of a 416 response we log it. This prevents the go reverse
   proxy from choking on it and turning it into a 502 Bad Gateway response.

   https://github.com/cs3org/reva/pull/1884

*   Enhancement #1744: Add support for lightweight user types

   This PR adds support for assigning and consuming user type when setting/reading users. On top
   of that, support for lightweight users is added. These users have to be restricted to accessing
   only shares received by them, which is accomplished by expanding the existing RBAC scope.

   https://github.com/cs3org/reva/pull/1744
   https://github.com/cs3org/cs3apis/pull/120

Changelog for reva 1.9.1 (2021-07-09)
=======================================

The following sections list the changes in reva 1.9.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder
*   Fix #1835: Cleanup owncloudsql driver
*   Fix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping
*   Fix #1885: Fix template in eoshomewrapper to use context user rather than resource
*   Fix #1833: Properly handle name collisions for deletes in the owncloud driver
*   Fix #1874: Use the original file mtime during upload
*   Fix #1854: Add the uid/gid to the url for eos
*   Fix #1848: Fill in missing gid/uid number with nobody
*   Fix #1831: Make the ocm-provider endpoint in the ocmd service unprotected
*   Fix #1808: Use empty array in OCS Notifications endpoints
*   Fix #1825: Raise max grpc message size
*   Fix #1828: Send a proper XML header with error messages
*   Chg #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7
*   Enh #1834: Add API key to Mentix GOCDB connector
*   Enh #1855: Minor optimization in parsing EOS ACLs
*   Enh #1873: Update the EOS image tag to be for revad-eos image
*   Enh #1802: Introduce list spaces
*   Enh #1849: Add readonly interceptor
*   Enh #1875: Simplify resource comparison
*   Enh #1827: Support trashbin sub paths in the recycle API

Details
-------

*   Bugfix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder

   This was introduced on https://github.com/cs3org/reva/commit/117adad while porting the
   configuration on .drone.yml to starlark.

   Force golang:alpine3.13 as base image to prevent errors from Make when running on Docker
   <20.10 as it happens on Drone
   ref.https://gitlab.alpinelinux.org/alpine/aports/-/issues/12396

   https://github.com/cs3org/reva/pull/1843
   https://github.com/cs3org/reva/pull/1844
   https://github.com/cs3org/reva/pull/1847

*   Bugfix #1835: Cleanup owncloudsql driver

   Use `owncloudsql` string when returning errors and removed copyMD as it does not need to copy
   metadata from files.

   https://github.com/cs3org/reva/pull/1835

*   Bugfix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping

   https://github.com/cs3org/reva/pull/1868

*   Bugfix #1885: Fix template in eoshomewrapper to use context user rather than resource

   https://github.com/cs3org/reva/pull/1885

*   Bugfix #1833: Properly handle name collisions for deletes in the owncloud driver

   In the owncloud storage driver when we delete a file we append the deletion time to the file name.
   If two fast consecutive deletes happened, the deletion time would be the same and if the two
   files had the same name we ended up with only one file in the trashbin.

   https://github.com/cs3org/reva/pull/1833

*   Bugfix #1874: Use the original file mtime during upload

   The decomposedfs was not using the original file mtime during uploads.

   https://github.com/cs3org/reva/pull/1874

*   Bugfix #1854: Add the uid/gid to the url for eos

   https://github.com/cs3org/reva/pull/1854

*   Bugfix #1848: Fill in missing gid/uid number with nobody

   When an LDAP server does not provide numeric uid or gid properties for a user we now fall back to a
   configurable `nobody` id (default 99).

   https://github.com/cs3org/reva/pull/1848

*   Bugfix #1831: Make the ocm-provider endpoint in the ocmd service unprotected

   https://github.com/cs3org/reva/issues/1751
   https://github.com/cs3org/reva/pull/1831

*   Bugfix #1808: Use empty array in OCS Notifications endpoints

   https://github.com/cs3org/reva/pull/1808

*   Bugfix #1825: Raise max grpc message size

   As a workaround for listing larger folder we raised the `MaxCallRecvMsgSize` to 10MB. This
   should be enough for ~15k files. The proper fix is implementing ListContainerStream in the
   gateway, but we needed a way to test the web ui with larger collections.

   https://github.com/cs3org/reva/pull/1825

*   Bugfix #1828: Send a proper XML header with error messages

   https://github.com/cs3org/reva/pull/1828

*   Change #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7

   In order to upgrade mattn/go-sqlite3 to v1.14.7, the odic provider service is removed, which
   is possible because it is not used anymore

   https://github.com/cs3org/reva/pull/1828
   https://github.com/owncloud/ocis/pull/2209

*   Enhancement #1834: Add API key to Mentix GOCDB connector

   The PI (programmatic interface) of the GOCDB will soon require an API key; this PR adds the
   ability to configure this key in Mentix.

   https://github.com/cs3org/reva/pull/1834

*   Enhancement #1855: Minor optimization in parsing EOS ACLs

   https://github.com/cs3org/reva/pull/1855

*   Enhancement #1873: Update the EOS image tag to be for revad-eos image

   https://github.com/cs3org/reva/pull/1873

*   Enhancement #1802: Introduce list spaces

   The ListStorageSpaces call now allows listing all user homes and shared resources using a
   storage space id. The gateway will forward requests to a specific storage provider when a
   filter by id is given. Otherwise it will query all storage providers. Results will be
   deduplicated. Currently, only the decomposed fs storage driver implements the necessary
   logic to demonstrate the implmentation. A new `/dav/spaces` WebDAV endpoint to directly
   access a storage space is introduced in a separate PR.

   https://github.com/cs3org/reva/pull/1802
   https://github.com/cs3org/reva/pull/1803

*   Enhancement #1849: Add readonly interceptor

   The readonly interceptor could be used to configure a storageprovider in readonly mode. This
   could be handy in some migration scenarios.

   https://github.com/cs3org/reva/pull/1849

*   Enhancement #1875: Simplify resource comparison

   We replaced ResourceEqual with ResourceIDEqual where possible.

   https://github.com/cs3org/reva/pull/1875

*   Enhancement #1827: Support trashbin sub paths in the recycle API

   The recycle API could only act on the root items of the trashbin. Meaning if you delete a deep
   tree, you couldn't restore just one file from that tree but you had to restore the whole tree. Now
   listing, restoring and purging work also for sub paths in the trashbin.

   https://github.com/cs3org/reva/pull/1827

Changelog for reva 1.9.0 (2021-06-23)
=======================================

The following sections list the changes in reva 1.9.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests
*   Fix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI
*   Chg #1721: Absolute and relative references
*   Enh #1810: Add arbitrary metadata support to EOS
*   Enh #1774: Add user ID cache warmup to EOS storage driver
*   Enh #1471: EOEGrpc progress. Logging discipline and error handling
*   Enh #1811: Harden public shares signing
*   Enh #1793: Remove the user id from the trashbin key
*   Enh #1795: Increase trashbin restore API compatibility
*   Enh #1516: Use UidNumber and GidNumber fields in User objects
*   Enh #1820: Tag v1.9.0

Details
-------

*   Bugfix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests

   Replace the backquotes that were used for the date component of the URL with the
   POSIX-confirmant command substitution '$()'.

   https://github.com/cs3org/reva/pull/1815

*   Bugfix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI

   Some of the dependencies used by revad need CGO to be enabled in order to work. We also need to
   install the 'mime-types' in alpine to correctly detect them on the storage-providers.

   The CGO_ENABLED=0 flag was added to the docker build flags so that it will produce a static
   build. This allows usage of the 'scratch' image for reduction of the docker image size (e.g. the
   reva cli).

   https://github.com/cs3org/reva/issues/1765
   https://github.com/cs3org/reva/pull/1766
   https://github.com/cs3org/reva/pull/1797

*   Change #1721: Absolute and relative references

   We unified the `Reference_Id` end `Reference_Path` types to a combined `Reference` that
   contains both: - a `resource_id` property that can identify a node using a `storage_id` and an
   `opaque_id` - a `path` property that can be used to represent absolute paths as well as paths
   relative to the id based properties. While this is a breaking change it allows passing both:
   absolute as well as relative references.

   https://github.com/cs3org/reva/pull/1721

*   Enhancement #1810: Add arbitrary metadata support to EOS

   https://github.com/cs3org/reva/pull/1810

*   Enhancement #1774: Add user ID cache warmup to EOS storage driver

   https://github.com/cs3org/reva/pull/1774

*   Enhancement #1471: EOEGrpc progress. Logging discipline and error handling

   https://github.com/cs3org/reva/pull/1471

*   Enhancement #1811: Harden public shares signing

   Makes golangci-lint happy as well

   https://github.com/cs3org/reva/pull/1811

*   Enhancement #1793: Remove the user id from the trashbin key

   We don't want to use the users uuid outside of the backend so I removed the id from the trashbin
   file key.

   https://github.com/cs3org/reva/pull/1793

*   Enhancement #1795: Increase trashbin restore API compatibility

   * The precondition were not checked before doing a trashbin restore in the ownCloud dav API.
   Without the checks the API would behave differently compared to the oC10 API. * The restore
   response was missing HTTP headers like `ETag` * Update the name when restoring the file from
   trashbin to a new target name

   https://github.com/cs3org/reva/pull/1795

*   Enhancement #1516: Use UidNumber and GidNumber fields in User objects

   Update instances where CS3API's `User` objects are created and used to use `GidNumber`, and
   `UidNumber` fields instead of storing them in `Opaque` map.

   https://github.com/cs3org/reva/issues/1516

*   Enhancement #1820: Tag v1.9.0

   Bump release number to v1.9.0 as it contains breaking changes related to changing the
   reference type.

   https://github.com/cs3org/reva/pull/1820

Changelog for reva 1.8.0 (2021-06-09)
=======================================

The following sections list the changes in reva 1.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1779: Set Content-Type header correctly for ocs requests
*   Fix #1650: Allow fetching shares as the grantee
*   Fix #1693: Fix move in owncloud storage driver
*   Fix #1666: Fix public file shares
*   Fix #1541: Allow for restoring recycle items to different locations
*   Fix #1718: Use the -static ldflag only for the 'build-ci' target
*   Enh #1719: Application passwords CLI
*   Enh #1719: Application passwords management
*   Enh #1725: Create transfer type share
*   Enh #1755: Return file checksum available from the metadata for the EOS driver
*   Enh #1673: Deprecate using errors.New and fmt.Errorf
*   Enh #1723: Open in app workflow using the new API
*   Enh #1655: Improve json marshalling of share protobuf messages
*   Enh #1694: User profile picture capability
*   Enh #1649: Add reliability calculations support to Mentix
*   Enh #1509: Named Service Registration
*   Enh #1643: Cache resources from share getter methods in OCS
*   Enh #1664: Add cache warmup strategy for OCS resource infos
*   Enh #1710: Owncloudsql storage driver
*   Enh #1705: Reduce the size of all the container images built on CI
*   Enh #1669: Mint scope-based access tokens for RBAC
*   Enh #1683: Filter created shares based on type in OCS
*   Enh #1763: Sort share entries alphabetically
*   Enh #1758: Warn user for not recommended go version
*   Enh #1747: Add checksum headers to tus preflight responses
*   Enh #1685: Add share to update response

Details
-------

*   Bugfix #1779: Set Content-Type header correctly for ocs requests

   Before this fix the `Content-Type` header was guessed by `w.Write` because `WriteHeader` was
   called to early. Now the `Content-Type` is set correctly and to the same values as in ownCloud 10

   https://github.com/owncloud/ocis/issues/1779

*   Bugfix #1650: Allow fetching shares as the grantee

   The json backend now allows a grantee to fetch a share by id.

   https://github.com/cs3org/reva/pull/1650

*   Bugfix #1693: Fix move in owncloud storage driver

   When moving a file or folder (includes renaming) the filepath in the cache didn't get updated
   which caused subsequent requests to `getpath` to fail.

   https://github.com/cs3org/reva/issues/1693
   https://github.com/cs3org/reva/issues/1696

*   Bugfix #1666: Fix public file shares

   Fixed stat requests and propfind responses for publicly shared files.

   https://github.com/cs3org/reva/pull/1666

*   Bugfix #1541: Allow for restoring recycle items to different locations

   The CS3 APIs specify a way to restore a recycle item to a different location than the original by
   setting the `restore_path` field in the `RestoreRecycleItemRequest`. This field had not
   been considered until now.

   https://github.com/cs3org/reva/pull/1541
   https://cs3org.github.io/cs3apis/

*   Bugfix #1718: Use the -static ldflag only for the 'build-ci' target

   It is not intended to statically link the generated binaries for local development workflows.
   This resulted on segmentation faults and compiller warnings.

   https://github.com/cs3org/reva/pull/1718

*   Enhancement #1719: Application passwords CLI

   This PR adds the CLI commands `token-list`, `token-create` and `token-remove` to manage
   tokens with limited scope on behalf of registered users.

   https://github.com/cs3org/reva/pull/1719

*   Enhancement #1719: Application passwords management

   This PR adds the functionality to generate authentication tokens with limited scope on behalf
   of registered users. These can be used in third party apps or in case primary user credentials
   cannot be submitted to other parties.

   https://github.com/cs3org/reva/issues/1714
   https://github.com/cs3org/reva/pull/1719
   https://github.com/cs3org/cs3apis/pull/127

*   Enhancement #1725: Create transfer type share

   `transfer-create` creates a share of type transfer.

   https://github.com/cs3org/reva/pull/1725

*   Enhancement #1755: Return file checksum available from the metadata for the EOS driver

   https://github.com/cs3org/reva/pull/1755

*   Enhancement #1673: Deprecate using errors.New and fmt.Errorf

   Previously we were using errors.New and fmt.Errorf to create errors. Now we use the errors
   defined in the errtypes package.

   https://github.com/cs3org/reva/issues/1673

*   Enhancement #1723: Open in app workflow using the new API

   This provides a new `open-in-app` command for the CLI and the implementation on the
   appprovider gateway service for the new API, including the option to specify the appplication
   to use, thus overriding the preconfigured one.

   https://github.com/cs3org/reva/pull/1723

*   Enhancement #1655: Improve json marshalling of share protobuf messages

   Protobuf oneof fields cannot be properly handled by the native json marshaller, and the
   protojson package can only handle proto messages. Previously, we were using a workaround of
   storing these oneof fields separately, which made the code inelegant. Now we marshal these
   messages as strings before marshalling them via the native json package.

   https://github.com/cs3org/reva/pull/1655

*   Enhancement #1694: User profile picture capability

   Based on feedback in the new ownCloud web frontend we want to omit trying to render user avatars
   images / profile pictures based on the backend capabilities. Now the OCS communicates a
   corresponding value.

   https://github.com/cs3org/reva/pull/1694

*   Enhancement #1649: Add reliability calculations support to Mentix

   To make reliability calculations possible, a new exporter has been added to Mentix that reads
   scheduled downtimes from the GOCDB and exposes it through Prometheus metrics.

   https://github.com/cs3org/reva/pull/1649

*   Enhancement #1509: Named Service Registration

   Move away from hardcoding service IP addresses and rely upon name resolution instead. It
   delegates the address lookup to a static in-memory service registry, which can be
   re-implemented in multiple forms.

   https://github.com/cs3org/reva/pull/1509

*   Enhancement #1643: Cache resources from share getter methods in OCS

   In OCS, once we retrieve the shares from the shareprovider service, we stat each of those
   separately to obtain the required info, which introduces a lot of latency. This PR introduces a
   resoource info cache in OCS, which would prevent this latency.

   https://github.com/cs3org/reva/pull/1643

*   Enhancement #1664: Add cache warmup strategy for OCS resource infos

   Recently, a TTL cache was added to OCS to store statted resource infos. This PR adds an interface
   to define warmup strategies and also adds a cbox specific strategy which starts a goroutine to
   initialize the cache with all the valid shares present in the system.

   https://github.com/cs3org/reva/pull/1664

*   Enhancement #1710: Owncloudsql storage driver

   This PR adds a storage driver which connects to a oc10 storage backend (storage + database).
   This allows for running oc10 and ocis with the same backend in parallel.

   https://github.com/cs3org/reva/pull/1710

*   Enhancement #1705: Reduce the size of all the container images built on CI

   Previously, all images were based on golang:1.16 which is built from Debian. Using 'scratch'
   as base, reduces the size of the artifacts well as the attack surface for all the images, plus
   copying the binary from the build step ensures that only the strictly required software is
   present on the final image. For the revad images tagged '-eos', eos-slim is used instead. It is
   still large but it updates the environment as well as the EOS version.

   https://github.com/cs3org/reva/pull/1705

*   Enhancement #1669: Mint scope-based access tokens for RBAC

   Primarily, this PR is meant to introduce the concept of scopes into our tokens. At the moment, it
   addresses those cases where we impersonate other users without allowing the full scope of what
   the actual user has access to.

   A short explanation for how it works for public shares: - We get the public share using the token
   provided by the client. - In the public share, we know the resource ID, so we can add this to the
   allowed scope, but not the path. - However, later OCDav tries to access by path as well. Now this
   is not allowed at the moment. However, from the allowed scope, we have the resource ID and we're
   allowed to stat that. We stat the resource ID, get the path and if the path matches the one passed
   by OCDav, we allow the request to go through.

   https://github.com/cs3org/reva/pull/1669

*   Enhancement #1683: Filter created shares based on type in OCS

   https://github.com/cs3org/reva/pull/1683

*   Enhancement #1763: Sort share entries alphabetically

   When showing the list of shares to the end-user, the list was not sorted alphabetically. This PR
   sorts the list of users and groups.

   https://github.com/cs3org/reva/issues/1763

*   Enhancement #1758: Warn user for not recommended go version

   This PR adds a warning while an user is building the source code, if he is using a go version not
   recommended.

   https://github.com/cs3org/reva/issues/1758
   https://github.com/cs3org/reva/pull/1760

*   Enhancement #1747: Add checksum headers to tus preflight responses

   Added `checksum` to the header `Tus-Extension` and added the `Tus-Checksum-Algorithm`
   header.

   https://github.com/owncloud/ocis/issues/1747
   https://github.com/cs3org/reva/pull/1702

*   Enhancement #1685: Add share to update response

   After accepting or rejecting a share the API includes the updated share in the response.

   https://github.com/cs3org/reva/pull/1685
   https://github.com/cs3org/reva/pull/1724

Changelog for reva 1.7.0 (2021-04-19)
=======================================

The following sections list the changes in reva 1.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1619: Fixes for enabling file sharing in EOS
*   Fix #1576: Fix etag changing only once a second
*   Fix #1634: Mentix site authorization status changes
*   Fix #1625: Make local file connector more error tolerant
*   Fix #1526: Fix webdav file versions endpoint bugs
*   Fix #1457: Cloning of internal mesh data lost some values
*   Fix #1597: Check for ENOTDIR on readlink error
*   Fix #1636: Skip file check for OCM data transfers
*   Fix #1552: Fix a bunch of trashbin related issues
*   Fix #1: Bump meshdirectory-web to 1.0.2
*   Chg #1562: Modularize api token management in GRAPPA drivers
*   Chg #1452: Separate blobs from metadata in the ocis storage driver
*   Enh #1514: Add grpc test suite for the storage provider
*   Enh #1466: Add integration tests for the s3ng driver
*   Enh #1521: Clarify expected failures
*   Enh #1624: Add wrappers for EOS and EOS Home storage drivers
*   Enh #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType
*   Enh #1411: Make InsecureSkipVerify configurable
*   Enh #1106: Make command to run litmus tests
*   Enh #1502: Bump meshdirectory-web to v1.0.4
*   Enh #1502: New MeshDirectory HTTP service UI frontend with project branding
*   Enh #1405: Quota querying and tree accounting
*   Enh #1527: Add FindAcceptedUsers method to OCM Invite API
*   Enh #1149: Add CLI Commands for OCM invitation workflow
*   Enh #1629: Implement checksums in the owncloud storage
*   Enh #1528: Port drone pipeline definition to starlark
*   Enh #110: Add signature authentication for public links
*   Enh #1495: SQL driver for the publicshare service
*   Enh #1588: Make the additional info attribute for shares configurable
*   Enh #1595: Add site account registration panel
*   Enh #1506: Site Accounts service for API keys
*   Enh #116: Enhance storage registry with virtual views and regular expressions
*   Enh #1513: Add stubs for storage spaces manipulation

Details
-------

*   Bugfix #1619: Fixes for enabling file sharing in EOS

   https://github.com/cs3org/reva/pull/1619

*   Bugfix #1576: Fix etag changing only once a second

   We fixed a problem with the owncloud storage driver only considering the mtime with a second
   resolution for the etag calculation.

   https://github.com/cs3org/reva/pull/1576

*   Bugfix #1634: Mentix site authorization status changes

   If a site changes its authorization status, Mentix did not update its internal data to reflect
   this change. This PR fixes this issue.

   https://github.com/cs3org/reva/pull/1634

*   Bugfix #1625: Make local file connector more error tolerant

   The local file connector caused Reva to throw an exception if the local file for storing site
   data couldn't be loaded. This PR changes this behavior so that only a warning is logged.

   https://github.com/cs3org/reva/pull/1625

*   Bugfix #1526: Fix webdav file versions endpoint bugs

   Etag and error code related bugs have been fixed for the webdav file versions endpoint and
   removed from the expected failures file.

   https://github.com/cs3org/reva/pull/1526

*   Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

*   Bugfix #1597: Check for ENOTDIR on readlink error

   The deconstructed storage driver now handles ENOTDIR errors when `node.Child()` is called
   for a path containing a path segment that is actually a file.

   https://github.com/owncloud/ocis/issues/1239
   https://github.com/cs3org/reva/pull/1597

*   Bugfix #1636: Skip file check for OCM data transfers

   https://github.com/cs3org/reva/pull/1636

*   Bugfix #1552: Fix a bunch of trashbin related issues

   Fixed these issues:

   - Complete: Deletion time in trash bin shows a wrong date - Complete: shared trash status code -
   Partly: invalid webdav responses for unauthorized requests. - Partly: href in trashbin
   PROPFIND response is wrong

   Complete means there are no expected failures left. Partly means there are some scenarios
   left.

   https://github.com/cs3org/reva/pull/1552

*   Bugfix #1: Bump meshdirectory-web to 1.0.2

   Updated meshdirectory-web mod to version 1.0.2 that contains fixes for OCM invite API links
   generation.

   https://github.com/sciencemesh/meshdirectory-web/pull/1

*   Change #1562: Modularize api token management in GRAPPA drivers

   This PR moves the duplicated api token management methods into a seperate utils package

   https://github.com/cs3org/reva/issues/1562

*   Change #1452: Separate blobs from metadata in the ocis storage driver

   We changed the ocis storage driver to keep the file content separate from the metadata by
   storing the blobs in a separate directory. This allows for using a different (potentially
   faster) storage for the metadata.

   **Note** This change makes existing ocis storages incompatible with the new code.

   We also streamlined the ocis and the s3ng drivers so that most of the code is shared between them.

   https://github.com/cs3org/reva/pull/1452

*   Enhancement #1514: Add grpc test suite for the storage provider

   A new test suite has been added which tests the grpc interface to the storage provider. It
   currently runs against the ocis and the owncloud storage drivers.

   https://github.com/cs3org/reva/pull/1514

*   Enhancement #1466: Add integration tests for the s3ng driver

   We extended the integration test suite to also run all tests against the s3ng driver.

   https://github.com/cs3org/reva/pull/1466

*   Enhancement #1521: Clarify expected failures

   Some features, while covered by the ownCloud 10 acceptance tests, will not be implmented for
   now: - blacklisted / ignored files, because ocis/reva don't need to blacklist `.htaccess`
   files - `OC-LazyOps` support was [removed from the
   clients](https://github.com/owncloud/client/pull/8398). We are thinking about [a state
   machine for uploads to properly solve that scenario and also list the state of files in progress
   in the web ui](https://github.com/owncloud/ocis/issues/214). The expected failures
   files now have a dedicated _Won't fix_ section for these items.

   https://github.com/owncloud/ocis/issues/214
   https://github.com/cs3org/reva/pull/1521
   https://github.com/owncloud/client/pull/8398

*   Enhancement #1624: Add wrappers for EOS and EOS Home storage drivers

   For CERNBox, we need the mount ID to be configured according to the owner of a resource. Setting
   this in the storageprovider means having different instances of this service to cater to
   different users, which does not scale. This driver forms a wrapper around the EOS driver and
   sets the mount ID according to a configurable mapping based on the owner of the resource.

   https://github.com/cs3org/reva/pull/1624

*   Enhancement #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType

   Interface method Share() in pkg/ocm/share/share.go now has a share type parameter.

   https://github.com/cs3org/reva/pull/1563

*   Enhancement #1411: Make InsecureSkipVerify configurable

   Add `InsecureSkipVerify` field to `metrics.Config` struct and update examples to include
   it.

   https://github.com/cs3org/reva/issues/1411

*   Enhancement #1106: Make command to run litmus tests

   This updates adds an extra make command to run litmus tests via make. `make litmus-test`
   executes the tests.

   https://github.com/cs3org/reva/issues/1106
   https://github.com/cs3org/reva/pull/1543

*   Enhancement #1502: Bump meshdirectory-web to v1.0.4

   Updated meshdirectory-web version to v.1.0.4 bringing multiple UX improvements in provider
   list and map.

   https://github.com/cs3org/reva/issues/1502
   https://github.com/sciencemesh/meshdirectory-web/pull/2
   https://github.com/sciencemesh/meshdirectory-web/pull/3

*   Enhancement #1502: New MeshDirectory HTTP service UI frontend with project branding

   We replaced the temporary version of web frontend of the mesh directory http service with a new
   redesigned & branded one. Because the new version is a more complex Vue SPA that contains image,
   css and other assets, it is now served from a binary package distribution that was generated
   using the [github.com/rakyll/statik](https://github.com/rakyll/statik) package. The
   `http.services.meshdirectory.static` config option was obsoleted by this change.

   https://github.com/cs3org/reva/issues/1502

*   Enhancement #1405: Quota querying and tree accounting

   The ocs api now returns the user quota for the users home storage. Furthermore, the ocis storage
   driver now reads the quota from the extended attributes of the user home or root node and
   implements tree size accounting. Finally, ocdav PROPFINDS now handle the
   `DAV:quota-used-bytes` and `DAV:quote-available-bytes` properties.

   https://github.com/cs3org/reva/pull/1405
   https://github.com/cs3org/reva/pull/1491

*   Enhancement #1527: Add FindAcceptedUsers method to OCM Invite API

   https://github.com/cs3org/reva/pull/1527

*   Enhancement #1149: Add CLI Commands for OCM invitation workflow

   This adds a couple of CLI commands, `ocm-invite-generate` and `ocm-invite-forward` to
   generate and forward ocm invitation tokens respectively.

   https://github.com/cs3org/reva/issues/1149

*   Enhancement #1629: Implement checksums in the owncloud storage

   Implemented checksums in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1629

*   Enhancement #1528: Port drone pipeline definition to starlark

   Having the pipeline definition as a starlark script instead of plain yaml greatly improves the
   flexibility and allows for removing lots of duplicated definitions.

   https://github.com/cs3org/reva/pull/1528

*   Enhancement #110: Add signature authentication for public links

   Implemented signature authentication for public links in addition to the existing password
   authentication. This allows web clients to efficiently download files from password
   protected public shares.

   https://github.com/cs3org/cs3apis/issues/110
   https://github.com/cs3org/reva/pull/1590

*   Enhancement #1495: SQL driver for the publicshare service

   https://github.com/cs3org/reva/pull/1495

*   Enhancement #1588: Make the additional info attribute for shares configurable

   AdditionalInfoAttribute can be configured via the `additional_info_attribute` key in the
   form of a Go template string. If not explicitly set, the default value is `{{.Mail}}`

   https://github.com/cs3org/reva/pull/1588

*   Enhancement #1595: Add site account registration panel

   This PR adds a site account registration panel to the site accounts service. It also removes
   site registration from the xcloud metrics driver.

   https://github.com/cs3org/reva/pull/1595

*   Enhancement #1506: Site Accounts service for API keys

   This update adds a new service to Reva that handles site accounts creation and management.
   Registered sites can be assigned an API key through a simple web interface which is also part of
   this service. This API key can then be used to identify a user and his/her associated (vendor or
   partner) site.

   Furthermore, Mentix was extended to make use of this new service. This way, all sites now have a
   stable and unique site ID that not only avoids ID collisions but also introduces a new layer of
   security (i.e., sites can only be modified or removed using the correct API key).

   https://github.com/cs3org/reva/pull/1506

*   Enhancement #116: Enhance storage registry with virtual views and regular expressions

   Add the functionality to the storage registry service to handle user requests for references
   which can span across multiple storage providers, particularly useful for cases where
   directories are sharded across providers or virtual views are expected.

   https://github.com/cs3org/cs3apis/pull/116
   https://github.com/cs3org/reva/pull/1570

*   Enhancement #1513: Add stubs for storage spaces manipulation

   This PR adds stubs for the storage space CRUD methods in the storageprovider service and makes
   the expired shares janitor configureable in the publicshares SQL driver.

   https://github.com/cs3org/reva/pull/1513

Changelog for reva 1.6.0 (2021-02-16)
=======================================

The following sections list the changes in reva 1.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1425: Align href URL encoding with oc10
*   Fix #1461: Fix public link webdav permissions
*   Fix #1457: Cloning of internal mesh data lost some values
*   Fix #1429: Purge non-empty dirs from trash-bin
*   Fix #1408: Get error status from trash-bin response
*   Enh #1451: Render additional share with in ocs sharing api
*   Enh #1424: We categorized the list of expected failures
*   Enh #1434: CERNBox REST driver for groupprovider service
*   Enh #1400: Checksum support
*   Enh #1431: Update npm packages to fix vulnerabilities
*   Enh #1415: Indicate in EOS containers that TUS is not supported
*   Enh #1402: Parse EOS sys ACLs to generate CS3 resource permissions
*   Enh #1477: Set quota when creating home directory in EOS
*   Enh #1416: Use updated etag of home directory even if it is cached
*   Enh #1478: Enhance error handling for grappa REST drivers
*   Enh #1453: Add functionality to share resources with groups
*   Enh #99: Add stubs and manager for groupprovider service
*   Enh #1462: Hash public share passwords
*   Enh #1464: LDAP driver for the groupprovider service
*   Enh #1430: Capture non-deterministic behavior on storages
*   Enh #1456: Fetch user groups in OIDC and LDAP backend
*   Enh #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore
*   Enh #1467: Align default location for xrdcopy binary

Details
-------

*   Bugfix #1425: Align href URL encoding with oc10

   We now use the same percent encoding for URLs in WebDAV href properties as ownCloud 10.

   https://github.com/owncloud/ocis/issues/1120
   https://github.com/owncloud/ocis/issues/1296
   https://github.com/owncloud/ocis/issues/1307
   https://github.com/cs3org/reva/pull/1425
   https://github.com/cs3org/reva/pull/1472

*   Bugfix #1461: Fix public link webdav permissions

   We now correctly render `oc:permissions` on the root collection of a publicly shared folder
   when it has more than read permissions.

   https://github.com/cs3org/reva/pull/1461

*   Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

*   Bugfix #1429: Purge non-empty dirs from trash-bin

   This wasn't possible before if the directory was not empty

   https://github.com/cs3org/reva/pull/1429

*   Bugfix #1408: Get error status from trash-bin response

   Previously the status code was gathered from the wrong response.

   https://github.com/cs3org/reva/pull/1408

*   Enhancement #1451: Render additional share with in ocs sharing api

   Recipients can now be distinguished by their email, which is rendered as additional info in the
   ocs api for share and file owners as well as share recipients.

   https://github.com/owncloud/ocis/issues/1190
   https://github.com/cs3org/reva/pull/1451

*   Enhancement #1424: We categorized the list of expected failures

   We categorized all expected failures into _File_ (Basic file management like up and download,
   move, copy, properties, trash, versions and chunking), _Sync_ (Synchronization features
   like etag propagation, setting mtime and locking files), _Share_ (File and sync features in a
   shared scenario), _User management_ (User and group management features) and _Other_ (API,
   search, favorites, config, capabilities, not existing endpoints, CORS and others). The
   [Review and fix the tests that have sharing step to work with
   ocis](https://github.com/owncloud/core/issues/38006) reference has been removed, as we
   now have the sharing category

   https://github.com/owncloud/core/issues/38006
   https://github.com/cs3org/reva/pull/1424

*   Enhancement #1434: CERNBox REST driver for groupprovider service

   https://github.com/cs3org/reva/pull/1434

*   Enhancement #1400: Checksum support

   We now support checksums on file uploads and PROPFIND results. On uploads, the ocdav service
   now forwards the `OC-Checksum` (and the similar TUS `Upload-Checksum`) header to the storage
   provider. We added an internal http status code that allows storage drivers to return checksum
   errors. On PROPFINDs, ocdav now renders the `<oc:checksum>` header in a bug compatible way for
   oc10 backward compatibility with existing clients. Finally, GET and HEAD requests now return
   the `OC-Checksum` header.

   https://github.com/owncloud/ocis/issues/1291
   https://github.com/owncloud/ocis/issues/1316
   https://github.com/cs3org/reva/pull/1400
   https://github.com/owncloud/core/pull/38304

*   Enhancement #1431: Update npm packages to fix vulnerabilities

   https://github.com/cs3org/reva/pull/1431

*   Enhancement #1415: Indicate in EOS containers that TUS is not supported

   The OCDAV propfind response previously hardcoded the TUS headers due to which clients such as
   phoenix used the TUS protocol for uploads, which EOS doesn't support. Now we pass this property
   as an opaque entry in the containers metadata.

   https://github.com/cs3org/reva/pull/1415

*   Enhancement #1402: Parse EOS sys ACLs to generate CS3 resource permissions

   https://github.com/cs3org/reva/pull/1402

*   Enhancement #1477: Set quota when creating home directory in EOS

   https://github.com/cs3org/reva/pull/1477

*   Enhancement #1416: Use updated etag of home directory even if it is cached

   We cache the home directory and shares folder etags as calculating these is an expensive
   process. But if these directories were updated after the previously calculated etag was
   cached, we can ignore this calculation and directly return the new one.

   https://github.com/cs3org/reva/pull/1416

*   Enhancement #1478: Enhance error handling for grappa REST drivers

   https://github.com/cs3org/reva/pull/1478

*   Enhancement #1453: Add functionality to share resources with groups

   https://github.com/cs3org/reva/pull/1453

*   Enhancement #99: Add stubs and manager for groupprovider service

   Recently, there was a separation of concerns with regard to users and groups in CS3APIs. This PR
   adds the required stubs and drivers for the group manager.

   https://github.com/cs3org/cs3apis/pull/99
   https://github.com/cs3org/cs3apis/pull/102
   https://github.com/cs3org/reva/pull/1358

*   Enhancement #1462: Hash public share passwords

   The share passwords were only base64 encoded. Added hashing using bcrypt with configurable
   hash cost.

   https://github.com/cs3org/reva/pull/1462

*   Enhancement #1464: LDAP driver for the groupprovider service

   https://github.com/cs3org/reva/pull/1464

*   Enhancement #1430: Capture non-deterministic behavior on storages

   As a developer creating/maintaining a storage driver I want to be able to validate the
   atomicity of all my storage driver operations. * Test for: Start 2 uploads, pause the first one,
   let the second one finish first, resume the first one at some point in time. Both uploads should
   finish. Needs to result in 2 versions, last finished is the most recent version. * Test for:
   Start 2 MKCOL requests with the same path, one needs to fail.

   https://github.com/cs3org/reva/pull/1430

*   Enhancement #1456: Fetch user groups in OIDC and LDAP backend

   https://github.com/cs3org/reva/pull/1456

*   Enhancement #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore

   We added a new storage driver (s3ng) which stores the file metadata on a local filesystem
   (reusing the decomposed filesystem of the ocis driver) and the actual content as blobs in any
   s3-compatible blobstore.

   https://github.com/cs3org/reva/pull/1429

*   Enhancement #1467: Align default location for xrdcopy binary

   https://github.com/cs3org/reva/pull/1467

Changelog for reva 1.5.1 (2021-01-19)
=======================================

The following sections list the changes in reva 1.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1401: Use the user in request for deciding the layout for non-home DAV requests
*   Fix #1413: Re-include the '.git' dir in the Docker images to pass the version tag
*   Fix #1399: Fix ocis trash-bin purge
*   Enh #1397: Bump the Copyright date to 2021
*   Enh #1398: Support site authorization status in Mentix
*   Enh #1393: Allow setting favorites, mtime and a temporary etag
*   Enh #1403: Support remote cloud gathering metrics

Details
-------

*   Bugfix #1401: Use the user in request for deciding the layout for non-home DAV requests

   For the incoming /dav/files/userID requests, we have different namespaces depending on
   whether the request is for the logged-in user's namespace or not. Since in the storage drivers,
   we specify the layout depending only on the user whose resources are to be accessed, this fails
   when a user wants to access another user's namespace when the storage provider depends on the
   logged in user's namespace. This PR fixes that.

   For example, consider the following case. The owncloud fs uses a layout {{substr 0 1
   .Id.OpaqueId}}/{{.Id.OpaqueId}}. The user einstein sends a request to access a resource
   shared with him, say /dav/files/marie/abcd, which should be allowed. However, based on the
   way we applied the layout, there's no way in which this can be translated to /m/marie/.

   https://github.com/cs3org/reva/pull/1401

*   Bugfix #1413: Re-include the '.git' dir in the Docker images to pass the version tag

   And git SHA to the release tool.

   https://github.com/cs3org/reva/pull/1413

*   Bugfix #1399: Fix ocis trash-bin purge

   Fixes the empty trash-bin functionality for ocis-storage

   https://github.com/owncloud/product/issues/254
   https://github.com/cs3org/reva/pull/1399

*   Enhancement #1397: Bump the Copyright date to 2021

   https://github.com/cs3org/reva/pull/1397

*   Enhancement #1398: Support site authorization status in Mentix

   This enhancement adds support for a site authorization status to Mentix. This way, sites
   registered via a web app can now be excluded until authorized manually by an administrator.

   Furthermore, Mentix now sets the scheme for Prometheus targets. This allows us to also support
   monitoring of sites that do not support the default HTTPS scheme.

   https://github.com/cs3org/reva/pull/1398

*   Enhancement #1393: Allow setting favorites, mtime and a temporary etag

   We now let the ocis driver persist favorites, set temporary etags and the mtime as arbitrary
   metadata.

   https://github.com/owncloud/ocis/issues/567
   https://github.com/cs3org/reva/issues/1394
   https://github.com/cs3org/reva/pull/1393

*   Enhancement #1403: Support remote cloud gathering metrics

   The current metrics package can only gather metrics either from json files. With this feature,
   the metrics can be gathered polling the http endpoints exposed by the owncloud/nextcloud
   sciencemesh apps.

   https://github.com/cs3org/reva/pull/1403

Changelog for reva 1.5.0 (2021-01-12)
=======================================

The following sections list the changes in reva 1.5.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1385: Run changelog check only if there are changes in the code
*   Fix #1333: Delete sdk unit tests
*   Fix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files
*   Fix #1338: Fix fd leaks
*   Fix #1343: Fix ocis move
*   Fix #551: Fix purging deleted files with the ocis storage
*   Fix #863: Fix dav api for trashbin
*   Fix #204: Fix the ocs share with me response
*   Fix #1351: Fix xattr.Remove error check for macOS
*   Fix #1320: Do not panic on remote.php/dav/files/
*   Fix #1379: Make Jaeger agent usable
*   Fix #1331: Fix capabilities response for multiple client versions
*   Fix #1281: When sharing via ocs look up user by username
*   Fix #1334: Handle removal of public shares by token or ID
*   Chg #990: Replace the user uuid with the username in ocs share responses
*   Enh #1350: Add auth protocol based on user agent
*   Enh #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors
*   Enh #1364: Remove expired Link on Get
*   Enh #1340: Add cache to store UID to UserID mapping in EOS
*   Enh #1154: Add support for the protobuf interface to eos metadata
*   Enh #1154: Merge-rebase from master 10/11/2020
*   Enh #1359: Add cache for calculated etags for home and shares directory
*   Enh #1321: Add support for multiple data transfer protocols
*   Enh #1324: Log expected errors with debug level
*   Enh #1351: Map errtypes to status
*   Enh #1347: Support property to enable health checking on a service
*   Enh #1332: Add import support to Mentix
*   Enh #1371: Use self-hosted Drone CI
*   Enh #1354: Map bad request and unimplement to http status codes
*   Enh #929: Include share types in ocs propfind responses
*   Enh #1328: Add CLI commands for public shares
*   Enh #1388: Support range header in GET requests
*   Enh #1361: Remove expired Link on Access
*   Enh #1386: Docker image for cs3org/revad:VERSION-eos
*   Enh #1368: Calculate and expose actual file permission set

Details
-------

*   Bugfix #1385: Run changelog check only if there are changes in the code

   https://github.com/cs3org/reva/pull/1385

*   Bugfix #1333: Delete sdk unit tests

   These depend on a remote server running reva and thus fail in case of version mismatches.

   https://github.com/cs3org/reva/pull/1333

*   Bugfix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files

   There was a regression in which we were not routing correctly to the right storage depending on
   the url.

   https://github.com/cs3org/reva/pull/1342

*   Bugfix #1338: Fix fd leaks

   There were some left over open file descriptors on simple.go.

   https://github.com/cs3org/reva/pull/1338

*   Bugfix #1343: Fix ocis move

   Use the old node id to build the target path for xattr updates.

   https://github.com/owncloud/ocis/issues/975
   https://github.com/cs3org/reva/pull/1343

*   Bugfix #551: Fix purging deleted files with the ocis storage

   The ocis storage could load the owner information of a deleted file. This caused the storage to
   not be able to purge deleted files.

   https://github.com/owncloud/ocis/issues/551

*   Bugfix #863: Fix dav api for trashbin

   The api was comparing the requested username to the userid.

   https://github.com/owncloud/ocis/issues/863

*   Bugfix #204: Fix the ocs share with me response

   The path of the files shared with me was incorrect.

   https://github.com/owncloud/product/issues/204
   https://github.com/cs3org/reva/pull/1346

*   Bugfix #1351: Fix xattr.Remove error check for macOS

   Previously, we checked the xattr.Remove error only for linux systems. Now macOS is checked
   also

   https://github.com/cs3org/reva/pull/1351

*   Bugfix #1320: Do not panic on remote.php/dav/files/

   Currently requests to /remote.php/dav/files/ result in panics since we cannot longer strip
   the user + destination from the url. This fixes the server response code and adds an error body to
   the response.

   https://github.com/cs3org/reva/pull/1320

*   Bugfix #1379: Make Jaeger agent usable

   Previously, you could not use tracing with jaeger agent because the tracing connector is
   always used instead of the tracing endpoint.

   This PR removes the defaults for collector and tracing endpoint.

   https://github.com/cs3org/reva/pull/1379

*   Bugfix #1331: Fix capabilities response for multiple client versions

   https://github.com/cs3org/reva/pull/1331

*   Bugfix #1281: When sharing via ocs look up user by username

   The ocs api returns usernames when listing share recipients, so the lookup when creating the
   share needs to search the usernames and not the userid.

   https://github.com/cs3org/reva/pull/1281

*   Bugfix #1334: Handle removal of public shares by token or ID

   Previously different drivers handled removing public shares using different means, either
   the token or the ID. Now, both the drivers support both these methods.

   https://github.com/cs3org/reva/pull/1334

*   Change #990: Replace the user uuid with the username in ocs share responses

   The ocs api should not send the users uuid. Replaced the uuid with the username.

   https://github.com/owncloud/ocis/issues/990
   https://github.com/cs3org/reva/pull/1375

*   Enhancement #1350: Add auth protocol based on user agent

   Previously, all available credential challenges are given to the client, for example, basic
   auth, bearer token, etc ... Different clients have different priorities to use one method or
   another, and before it was not possible to force a client to use one method without having a side
   effect on other clients.

   This PR adds the functionality to target a specific auth protocol based on the user agent HTTP
   header.

   https://github.com/cs3org/reva/pull/1350

*   Enhancement #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors

   Prevent the errors while storing new 'daily' releases from going unnoticed on the CI.

   https://github.com/cs3org/reva/pull/1362

*   Enhancement #1364: Remove expired Link on Get

   There is the scenario in which a public link has expired but ListPublicLink has not run,
   accessing a technically expired public share is still possible.

   https://github.com/cs3org/reva/pull/1364

*   Enhancement #1340: Add cache to store UID to UserID mapping in EOS

   Previously, we used to send an RPC to the user provider service for every lookup of user IDs from
   the UID stored in EOS. This PR adds an in-memory lock-protected cache to store this mapping.

   https://github.com/cs3org/reva/pull/1340

*   Enhancement #1154: Add support for the protobuf interface to eos metadata

   https://github.com/cs3org/reva/pull/1154

*   Enhancement #1154: Merge-rebase from master 10/11/2020

   https://github.com/cs3org/reva/pull/1154

*   Enhancement #1359: Add cache for calculated etags for home and shares directory

   Since we store the references in the shares directory instead of actual resources, we need to
   calculate the etag on every list/stat call. This is rather expensive so adding a cache would
   help to a great extent with regard to the performance.

   https://github.com/cs3org/reva/pull/1359

*   Enhancement #1321: Add support for multiple data transfer protocols

   Previously, we had to configure which data transfer protocol to use in the dataprovider
   service. A previous PR added the functionality to redirect requests to different handlers
   based on the request method but that would lead to conflicts if multiple protocols don't
   support mutually exclusive sets of requests. This PR adds the functionality to have multiple
   such handlers simultaneously and the client can choose which protocol to use.

   https://github.com/cs3org/reva/pull/1321
   https://github.com/cs3org/reva/pull/1285/

*   Enhancement #1324: Log expected errors with debug level

   While trying to download a non existing file and reading a non existing attribute are
   technically an error they are to be expected and nothing an admin can or even should act upon.

   https://github.com/cs3org/reva/pull/1324

*   Enhancement #1351: Map errtypes to status

   When mapping errtypes to grpc statuses we now also map bad request and not implemented /
   unsupported cases in the gateway storageprovider.

   https://github.com/cs3org/reva/pull/1351

*   Enhancement #1347: Support property to enable health checking on a service

   This update introduces a new service property called `ENABLE_HEALTH_CHECKS` that must be
   explicitly set to `true` if a service should be checked for its health status. This allows us to
   only enable these checks for partner sites only, skipping vendor sites.

   https://github.com/cs3org/reva/pull/1347

*   Enhancement #1332: Add import support to Mentix

   This update adds import support to Mentix, transforming it into a **Mesh Entity Exchanger**.
   To properly support vendor site management, a new connector that works on a local file has been
   added as well.

   https://github.com/cs3org/reva/pull/1332

*   Enhancement #1371: Use self-hosted Drone CI

   Previously, we used the drone cloud to run the CI for the project. Due to unexpected and sudden
   stop of the service for the cs3org we decided to self-host it.

   https://github.com/cs3org/reva/pull/1371

*   Enhancement #1354: Map bad request and unimplement to http status codes

   We now return a 400 bad request when a grpc call fails with an invalid argument status and a 501 not
   implemented when it fails with an unimplemented status. This prevents 500 errors when a user
   tries to add resources to the Share folder or a storage does not implement an action.

   https://github.com/cs3org/reva/pull/1354

*   Enhancement #929: Include share types in ocs propfind responses

   Added the share types to the ocs propfind response when a resource has been shared.

   https://github.com/owncloud/ocis/issues/929
   https://github.com/cs3org/reva/pull/1329

*   Enhancement #1328: Add CLI commands for public shares

   https://github.com/cs3org/reva/pull/1328

*   Enhancement #1388: Support range header in GET requests

   To allow resuming a download we now support GET requests with a range header.

   https://github.com/owncloud/ocis-reva/issues/12
   https://github.com/cs3org/reva/pull/1388

*   Enhancement #1361: Remove expired Link on Access

   Since there is no background jobs scheduled to wipe out expired resources, for the time being
   public links are going to be removed on a "on demand" basis, meaning whenever there is an API call
   that access the list of shares for a given resource, we will check whether the share is expired
   and delete it if so.

   https://github.com/cs3org/reva/pull/1361

*   Enhancement #1386: Docker image for cs3org/revad:VERSION-eos

   Based on eos:c8_4.8.15 (Centos8, version 4.8.15). To be used when the Reva daemon needs IPC
   with xrootd/eos via stdin/out.

   https://github.com/cs3org/reva/pull/1386
   https://github.com/cs3org/reva/pull/1389

*   Enhancement #1368: Calculate and expose actual file permission set

   Instead of hardcoding the permissions set for every file and folder to ListContainer:true,
   CreateContainer:true and always reporting the hardcoded string WCKDNVR for the WebDAV
   permissions we now aggregate the actual cs3 resource permission set in the storage drivers and
   correctly map them to ocs permissions and webdav permissions using a common role struct that
   encapsulates the mapping logic.

   https://github.com/owncloud/ocis/issues/552
   https://github.com/owncloud/ocis/issues/762
   https://github.com/owncloud/ocis/issues/763
   https://github.com/owncloud/ocis/issues/893
   https://github.com/owncloud/ocis/issues/1126
   https://github.com/owncloud/ocis-reva/issues/47
   https://github.com/owncloud/ocis-reva/issues/315
   https://github.com/owncloud/ocis-reva/issues/316
   https://github.com/owncloud/product/issues/270
   https://github.com/cs3org/reva/pull/1368

Changelog for reva 1.4.0 (2020-11-17)
=======================================

The following sections list the changes in reva 1.4.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1316: Fix listing shares for nonexisting path
*   Fix #1274: Let the gateway filter invalid references
*   Fix #1269: Handle more eos errors
*   Fix #1297: Check the err and the response status code
*   Fix #1260: Fix file descriptor leak on ocdav put handler
*   Fix #1253: Upload file to storage provider after assembling chunks
*   Fix #1264: Fix etag propagation in ocis driver
*   Fix #1255: Check current node when iterating over path segments
*   Fix #1265: Stop setting propagation xattr on new files
*   Fix #260: Filter share with me requests
*   Fix #1317: Prevent nil pointer when listing shares
*   Fix #1259: Fix propfind response code on forbidden files
*   Fix #1294: Fix error type in read node when file was not found
*   Fix #1258: Update share grants on share update
*   Enh #1257: Add a test user to all sites
*   Enh #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect
*   Enh #1239: Add logic for finding groups to user provider service
*   Enh #1280: Add a Reva SDK
*   Enh #1237: Setup of grpc transfer service and cli
*   Enh #1224: Add SQL driver for share manager
*   Enh #1285: Refactor the uploading files workflow from various clients
*   Enh #1233: Add support for custom CodiMD mimetype

Details
-------

*   Bugfix #1316: Fix listing shares for nonexisting path

   When trying to list shares for a not existing file or folder the ocs sharing implementation no
   longer responds with the wrong status code and broken xml.

   https://github.com/cs3org/reva/pull/1316

*   Bugfix #1274: Let the gateway filter invalid references

   We now filter deleted and unshared entries from the response when listing the shares folder of a
   user.

   https://github.com/cs3org/reva/pull/1274

*   Bugfix #1269: Handle more eos errors

   We now treat E2BIG, EACCES as a permission error, which occur, eg. when acl checks fail and
   return a permission denied error.

   https://github.com/cs3org/reva/pull/1269

*   Bugfix #1297: Check the err and the response status code

   The publicfile handler needs to check the response status code to return proper not pound and
   permission errors in the webdav api.

   https://github.com/cs3org/reva/pull/1297

*   Bugfix #1260: Fix file descriptor leak on ocdav put handler

   File descriptors on the ocdav service, especially on the put handler was leaking http
   connections. This PR addresses this.

   https://github.com/cs3org/reva/pull/1260

*   Bugfix #1253: Upload file to storage provider after assembling chunks

   In the PUT handler for chunked uploads in ocdav, we store the individual chunks in temporary
   file but do not write the assembled file to storage. This PR fixes that.

   https://github.com/cs3org/reva/pull/1253

*   Bugfix #1264: Fix etag propagation in ocis driver

   We now use a new synctime timestamp instead of trying to read the mtime to avoid race conditions
   when the stat request happens too quickly.

   https://github.com/owncloud/product/issues/249
   https://github.com/cs3org/reva/pull/1264

*   Bugfix #1255: Check current node when iterating over path segments

   When checking permissions we were always checking the leaf instead of using the current node
   while iterating over path segments.

   https://github.com/cs3org/reva/pull/1255

*   Bugfix #1265: Stop setting propagation xattr on new files

   We no longer set the propagation flag on a file because it is only evaluated for folders anyway.

   https://github.com/cs3org/reva/pull/1265

*   Bugfix #260: Filter share with me requests

   The OCS API now properly filters share with me requests by path and by share status (pending,
   accepted, rejected, all)

   https://github.com/owncloud/ocis-reva/issues/260
   https://github.com/owncloud/ocis-reva/issues/311
   https://github.com/cs3org/reva/pull/1301

*   Bugfix #1317: Prevent nil pointer when listing shares

   We now handle cases where the grpc connection failed correctly by no longer trying to access the
   response status.

   https://github.com/cs3org/reva/pull/1317

*   Bugfix #1259: Fix propfind response code on forbidden files

   When executing a propfind to a resource owned by another user the service would respond with a
   HTTP 403. In ownCloud 10 the response was HTTP 207. This change sets the response code to HTTP 207
   to stay backwards compatible.

   https://github.com/cs3org/reva/pull/1259

*   Bugfix #1294: Fix error type in read node when file was not found

   The method ReadNode in the ocis storage didn't return the error type NotFound when a file was not
   found.

   https://github.com/cs3org/reva/pull/1294

*   Bugfix #1258: Update share grants on share update

   When a share was updated the share information in the share manager was updated but the grants
   set by the storage provider were not.

   https://github.com/cs3org/reva/pull/1258

*   Enhancement #1257: Add a test user to all sites

   For health monitoring of all mesh sites, we need a special user account that is present on every
   site. This PR adds such a user to each users-*.json file so that every site will have the same test
   user credentials.

   https://github.com/cs3org/reva/pull/1257

*   Enhancement #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect

   Applications served by the WOPI bridge (CodiMD for the time being) require an extra
   redirection as the WOPI bridge itself behaves like a user app. This change returns to the client
   the redirected URL from the WOPI bridge, which is the real application URL.

   https://github.com/cs3org/reva/pull/1234

*   Enhancement #1239: Add logic for finding groups to user provider service

   To create shares with user groups, the functionality for searching for these based on a pattern
   is needed. This PR adds that.

   https://github.com/cs3org/reva/pull/1239

*   Enhancement #1280: Add a Reva SDK

   A Reva SDK has been added to make working with a remote Reva instance much easier by offering a
   high-level API that hides all the underlying details of the CS3API.

   https://github.com/cs3org/reva/pull/1280

*   Enhancement #1237: Setup of grpc transfer service and cli

   The grpc transfer service and cli for it.

   https://github.com/cs3org/reva/pull/1237

*   Enhancement #1224: Add SQL driver for share manager

   This PR adds an SQL driver for the shares manager which expects a schema equivalent to the one
   used in production for CERNBox.

   https://github.com/cs3org/reva/pull/1224

*   Enhancement #1285: Refactor the uploading files workflow from various clients

   Previously, we were implementing the tus client logic in the ocdav service, leading to
   restricting the whole of tus logic to the internal services. This PR refactors that workflow to
   accept incoming requests following the tus protocol while using simpler transmission
   internally.

   https://github.com/cs3org/reva/pull/1285
   https://github.com/cs3org/reva/pull/1314

*   Enhancement #1233: Add support for custom CodiMD mimetype

   The new mimetype is associated with the `.zmd` file extension. The corresponding
   configuration is associated with the storageprovider.

   https://github.com/cs3org/reva/pull/1233
   https://github.com/cs3org/reva/pull/1284

Changelog for reva 1.3.0 (2020-10-08)
=======================================

The following sections list the changes in reva 1.3.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1140: Call the gateway stat method from appprovider
*   Fix #1170: Up and download of file shares
*   Fix #1177: Fix ocis move
*   Fix #1178: Fix litmus failing on ocis storage
*   Fix #237: Fix missing quotes on OCIS-Storage
*   Fix #1210: No longer swallow permissions errors in the gateway
*   Fix #1183: Handle eos EPERM as permission denied
*   Fix #1206: No longer swallow permissions errors
*   Fix #1207: No longer swallow permissions errors in ocdav
*   Fix #1161: Cache display names in ocs service
*   Fix #1216: Add error handling for invalid references
*   Enh #1205: Allow using the username when accessing the users home
*   Enh #1131: Use updated cato to display nested package config in parent docs
*   Enh #1213: Check permissions in ocis driver
*   Enh #1202: Check permissions in owncloud driver
*   Enh #1228: Add GRPC stubs for CreateSymlink method
*   Enh #1174: Add logic in EOS FS for maintaining same inode across file versions
*   Enh #1142: Functionality to map home directory to different storage providers
*   Enh #1190: Add Blackbox Exporter support to Mentix
*   Enh #1229: New gateway datatx service
*   Enh #1225: Allow setting the owner when using the ocis driver
*   Enh #1180: Introduce ocis driver treetime accounting
*   Enh #1208: Calculate etags on-the-fly for shares directory and home folder

Details
-------

*   Bugfix #1140: Call the gateway stat method from appprovider

   The appprovider service used to directly pass the stat request to the storage provider
   bypassing the gateway, which resulted in errors while handling share children as they are
   resolved in the gateway path.

   https://github.com/cs3org/reva/pull/1140

*   Bugfix #1170: Up and download of file shares

   The shared folder logic in the gateway storageprovider was not allowing file up and downloads
   for single file shares. We now check if the reference is actually a file to determine if up /
   download should be allowed.

   https://github.com/cs3org/reva/pull/1170

*   Bugfix #1177: Fix ocis move

   When renaming a file we updating the name attribute on the wrong node, causing the path
   construction to use the wrong name. This fixes the litmus move_coll test.

   https://github.com/cs3org/reva/pull/1177

*   Bugfix #1178: Fix litmus failing on ocis storage

   We now ignore the `no data available` error when removing a non existing metadata attribute,
   which is ok because we are trying to delete it anyway.

   https://github.com/cs3org/reva/issues/1178
   https://github.com/cs3org/reva/pull/1179

*   Bugfix #237: Fix missing quotes on OCIS-Storage

   Etags have to be enclosed in quotes ". Return correct etags on OCIS-Storage.

   https://github.com/owncloud/product/issues/237
   https://github.com/cs3org/reva/pull/1232

*   Bugfix #1210: No longer swallow permissions errors in the gateway

   The gateway is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `status.NewPermissionDenied` or `status.NewInternal` instead of reusing the original
   response status.

   https://github.com/cs3org/reva/pull/1210

*   Bugfix #1183: Handle eos EPERM as permission denied

   We now treat EPERM errors, which occur, eg. when acl checks fail and return a permission denied
   error.

   https://github.com/cs3org/reva/pull/1183

*   Bugfix #1206: No longer swallow permissions errors

   The storageprovider is no longer ignoring permissions errors. It will now report them
   properly using `status.NewPermissionDenied(...)` instead of `status.NewInternal(...)`

   https://github.com/cs3org/reva/pull/1206

*   Bugfix #1207: No longer swallow permissions errors in ocdav

   The ocdav api is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `http.StatusForbidden` instead of `http.StatusInternalServerError`

   https://github.com/cs3org/reva/pull/1207

*   Bugfix #1161: Cache display names in ocs service

   The ocs list shares endpoint may need to fetch the displayname for multiple different users. We
   are now caching the lookup fo 60 seconds to save redundant RPCs to the users service.

   https://github.com/cs3org/reva/pull/1161

*   Bugfix #1216: Add error handling for invalid references

   https://github.com/cs3org/reva/pull/1216
   https://github.com/cs3org/reva/pull/1218

*   Enhancement #1205: Allow using the username when accessing the users home

   We now allow using the userid and the username when accessing the users home on the `/dev/files`
   endpoint.

   https://github.com/cs3org/reva/pull/1205

*   Enhancement #1131: Use updated cato to display nested package config in parent docs

   Previously, in case of nested packages, we just had a link pointing to the child package. Now we
   copy the nested package's documentation to the parent itself to make it easier for devs.

   https://github.com/cs3org/reva/pull/1131

*   Enhancement #1213: Check permissions in ocis driver

   We are now checking grant permissions in the ocis storage driver.

   https://github.com/cs3org/reva/pull/1213

*   Enhancement #1202: Check permissions in owncloud driver

   We are now checking grant permissions in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1202

*   Enhancement #1228: Add GRPC stubs for CreateSymlink method

   https://github.com/cs3org/reva/pull/1228

*   Enhancement #1174: Add logic in EOS FS for maintaining same inode across file versions

   This PR adds the functionality to maintain the same inode across various versions of a file by
   returning the inode of the version folder which remains constant. It requires extra metadata
   operations so a flag is provided to disable it.

   https://github.com/cs3org/reva/pull/1174

*   Enhancement #1142: Functionality to map home directory to different storage providers

   We hardcode the home path for all users to /home. This forbids redirecting requests for
   different users to multiple storage providers. This PR provides the option to map the home
   directories of different users using user attributes.

   https://github.com/cs3org/reva/pull/1142

*   Enhancement #1190: Add Blackbox Exporter support to Mentix

   This update extends Mentix to export a Prometheus SD file specific to the Blackbox Exporter
   which will be used for initial health monitoring. Usually, Prometheus requires its targets to
   only consist of the target's hostname; the BBE though expects a full URL here. This makes
   exporting two distinct files necessary.

   https://github.com/cs3org/reva/pull/1190

*   Enhancement #1229: New gateway datatx service

   Represents the CS3 datatx module in the gateway.

   https://github.com/cs3org/reva/pull/1229

*   Enhancement #1225: Allow setting the owner when using the ocis driver

   To support the metadata storage we allow setting the owner of the root node so that subsequent
   requests with that owner can be used to manage the storage.

   https://github.com/cs3org/reva/pull/1225

*   Enhancement #1180: Introduce ocis driver treetime accounting

   We added tree time accounting to the ocis storage driver which is modeled after [eos synctime
   accounting](http://eos-docs.web.cern.ch/eos-docs/configuration/namespace.html#enable-subtree-accounting).
   It can be enabled using the new `treetime_accounting` option, which defaults to `false` The
   `tmtime` is stored in an extended attribute `user.ocis.tmtime`. The treetime accounting is
   enabled for nodes which have the `user.ocis.propagation` extended attribute set to `"1"`.
   Currently, propagation is in sync.

   https://github.com/cs3org/reva/pull/1180

*   Enhancement #1208: Calculate etags on-the-fly for shares directory and home folder

   We create references for accepted shares in the shares directory, but these aren't updated
   when the original resource is modified. This PR adds the functionality to generate the etag for
   the shares directory and correspondingly, the home directory, based on the actual resources
   which the references point to, enabling the sync functionality.

   https://github.com/cs3org/reva/pull/1208

Changelog for reva 1.2.1 (2020-09-15)
=======================================

The following sections list the changes in reva 1.2.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1124: Do not swallow 'not found' errors in Stat
*   Enh #1125: Rewire dav files to the home storage
*   Enh #559: Introduce ocis storage driver
*   Enh #1118: Metrics module can be configured to retrieve metrics data from file

Details
-------

*   Bugfix #1124: Do not swallow 'not found' errors in Stat

   Webdav needs to determine if a file exists to return 204 or 201 response codes. When stating a non
   existing resource the NOT_FOUND code was replaced with an INTERNAL error code. This PR passes
   on a NOT_FOUND status code in the gateway.

   https://github.com/cs3org/reva/pull/1124

*   Enhancement #1125: Rewire dav files to the home storage

   If the user specified in the dav files URL matches the current one, rewire it to use the
   webDavHandler which is wired to the home storage.

   This fixes path mapping issues.

   https://github.com/cs3org/reva/pull/1125

*   Enhancement #559: Introduce ocis storage driver

   We introduced a now storage driver `ocis` that deconstructs a filesystem and uses a node first
   approach to implement an efficient lookup of files by path as well as by file id.

   https://github.com/cs3org/reva/pull/559

*   Enhancement #1118: Metrics module can be configured to retrieve metrics data from file

   - Export site metrics in Prometheus #698

   https://github.com/cs3org/reva/pull/1118

Changelog for reva 1.2.0 (2020-08-25)
=======================================

The following sections list the changes in reva 1.2.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1099: Do not restore recycle entry on purge
*   Fix #1091: Allow listing the trashbin
*   Fix #1103: Restore and delete trash items via ocs
*   Fix #1090: Ensure ignoring public stray shares
*   Fix #1064: Ensure ignoring stray shares
*   Fix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient
*   Fix #1115: Owncloud driver - propagate mtime on RemoveGrant
*   Fix #1111: Handle redirection prefixes when extracting destination from URL
*   Enh #1101: Add UID and GID in ldap auth driver
*   Enh #1077: Add calens check to verify changelog entries in CI
*   Enh #1072: Refactor Reva CLI with prompts
*   Enh #1079: Get file info using fxids from EOS
*   Enh #1088: Update LDAP user driver
*   Enh #1114: System information metrics cleanup
*   Enh #1071: System information included in Prometheus metrics
*   Enh #1094: Add logic for resolving storage references over webdav

Details
-------

*   Bugfix #1099: Do not restore recycle entry on purge

   This PR fixes a bug in the EOSFS driver that was restoring a deleted entry when asking for its
   permanent purge. EOS does not have the functionality to purge individual files, but the whole
   recycle of the user.

   https://github.com/cs3org/reva/pull/1099

*   Bugfix #1091: Allow listing the trashbin

   The trashbin endpoint expects the userid, not the username.

   https://github.com/cs3org/reva/pull/1091

*   Bugfix #1103: Restore and delete trash items via ocs

   The OCS api was not passing the correct key and references to the CS3 API. Furthermore, the
   owncloud storage driver was constructing the wrong target path when restoring.

   https://github.com/cs3org/reva/pull/1103

*   Bugfix #1090: Ensure ignoring public stray shares

   When using the json public shares manager, it can be the case we found a share with a resource_id
   that no longer exists.

   https://github.com/cs3org/reva/pull/1090

*   Bugfix #1064: Ensure ignoring stray shares

   When using the json shares manager, it can be the case we found a share with a resource_id that no
   longer exists. This PR aims to address his case by changing the contract of getPath and return
   the result of the STAT call instead of a generic error, so we can check the cause.

   https://github.com/cs3org/reva/pull/1064

*   Bugfix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient

   https://github.com/cs3org/reva/pull/1082
   https://github.com/cs3org/reva/pull/1116

*   Bugfix #1115: Owncloud driver - propagate mtime on RemoveGrant

   When removing a grant the mtime change also needs to be propagated. Only affectsn the owncluod
   storage driver.

   https://github.com/cs3org/reva/pull/1115

*   Bugfix #1111: Handle redirection prefixes when extracting destination from URL

   The move function handler in ocdav extracts the destination path from the URL by removing the
   base URL prefix from the URL path. This would fail in case there is a redirection prefix. This PR
   takes care of that and it also allows zero-size uploads for localfs.

   https://github.com/cs3org/reva/pull/1111

*   Enhancement #1101: Add UID and GID in ldap auth driver

   The PR https://github.com/cs3org/reva/pull/1088/ added the functionality to lookup UID
   and GID from the ldap user provider. This PR adds the same to the ldap auth manager.

   https://github.com/cs3org/reva/pull/1101

*   Enhancement #1077: Add calens check to verify changelog entries in CI

   https://github.com/cs3org/reva/pull/1077

*   Enhancement #1072: Refactor Reva CLI with prompts

   The current CLI is a bit cumbersome to use with users having to type commands containing all the
   associated config with no prompts or auto-completes. This PR refactors the CLI with these
   functionalities.

   https://github.com/cs3org/reva/pull/1072

*   Enhancement #1079: Get file info using fxids from EOS

   This PR supports getting file information from EOS using the fxid value.

   https://github.com/cs3org/reva/pull/1079

*   Enhancement #1088: Update LDAP user driver

   The LDAP user driver can now fetch users by a single claim / attribute. Use an `attributefilter`
   like `(&(objectclass=posixAccount)({{attr}}={{value}}))` in the driver section.

   It also adds the uid and gid to the users opaque properties so that eos can use them for chown and
   acl operations.

   https://github.com/cs3org/reva/pull/1088

*   Enhancement #1114: System information metrics cleanup

   The system information metrics are now based on OpenCensus instead of the Prometheus client
   library. Furthermore, its initialization was moved out of the Prometheus HTTP service to keep
   things clean.

   https://github.com/cs3org/reva/pull/1114

*   Enhancement #1071: System information included in Prometheus metrics

   System information is now included in the main Prometheus metrics exposed at `/metrics`.

   https://github.com/cs3org/reva/pull/1071

*   Enhancement #1094: Add logic for resolving storage references over webdav

   This PR adds the functionality to resolve webdav references using the ocs webdav service by
   passing the resource's owner's token. This would need to be changed in production.

   https://github.com/cs3org/reva/pull/1094

Changelog for reva 1.1.0 (2020-08-11)
=======================================

The following sections list the changes in reva 1.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #1069: Pass build time variables while compiling
*   Fix #1047: Fix missing idp check in GetUser of demo userprovider
*   Fix #1038: Do not stat shared resources when downloading
*   Fix #1034: Fixed some error reporting strings and corresponding logs
*   Fix #1046: Fixed resolution of fileid in GetPathByID
*   Fix #1052: Ocfs: Lookup user to render template properly
*   Fix #1024: Take care of trailing slashes in OCM package
*   Fix #1025: Use lower-case name for changelog directory
*   Fix #1042: List public shares only created by the current user
*   Fix #1051: Disallow sharing the shares directory
*   Enh #1035: Refactor AppProvider workflow
*   Enh #1059: Improve timestamp precision while logging
*   Enh #1037: System information HTTP service
*   Enh #995: Add UID and GID to the user object from user package

Details
-------

*   Bugfix #1069: Pass build time variables while compiling

   We provide the option of viewing various configuration and version options in both reva CLI as
   well as the reva daemon, but we didn't actually have these values in the first place. This PR adds
   that info at compile time.

   https://github.com/cs3org/reva/pull/1069

*   Bugfix #1047: Fix missing idp check in GetUser of demo userprovider

   We've added a check for matching idp in the GetUser function of the demo userprovider

   https://github.com/cs3org/reva/issues/1047

*   Bugfix #1038: Do not stat shared resources when downloading

   Previously, we statted the resources in all download requests resulting in failures when
   downloading references. This PR fixes that by statting only in case the resource is not present
   in the shares folder. It also fixes a bug where we allowed uploading to the mount path, resulting
   in overwriting the user home directory.

   https://github.com/cs3org/reva/pull/1038

*   Bugfix #1034: Fixed some error reporting strings and corresponding logs

   https://github.com/cs3org/reva/pull/1034

*   Bugfix #1046: Fixed resolution of fileid in GetPathByID

   Following refactoring of fileid generations in the local storage provider, this ensures
   fileid to path resolution works again.

   https://github.com/cs3org/reva/pull/1046

*   Bugfix #1052: Ocfs: Lookup user to render template properly

   Currently, the username is used to construct paths, which breaks when mounting the `owncloud`
   storage driver at `/oc` and then expecting paths that use the username like
   `/oc/einstein/foo` to work, because they will mismatch the path that is used from propagation
   which uses `/oc/u-u-i-d` as the root, giving an `internal path outside root` error

   https://github.com/cs3org/reva/pull/1052

*   Bugfix #1024: Take care of trailing slashes in OCM package

   Previously, we assumed that the OCM endpoints would have trailing slashes, failing in case
   they didn't. This PR fixes that.

   https://github.com/cs3org/reva/pull/1024

*   Bugfix #1025: Use lower-case name for changelog directory

   When preparing a new release, the changelog entries need to be copied to the changelog folder
   under docs. In a previous change, all these folders were made to have lower case names,
   resulting in creation of a separate folder.

   https://github.com/cs3org/reva/pull/1025

*   Bugfix #1042: List public shares only created by the current user

   When running ocis, the public links created by a user are visible to all the users under the
   'Shared with others' tab. This PR fixes that by returning only those links which are created by a
   user themselves.

   https://github.com/cs3org/reva/pull/1042

*   Bugfix #1051: Disallow sharing the shares directory

   Previously, it was possible to create public links for and share the shares directory itself.
   However, when the recipient tried to accept the share, it failed. This PR prevents the creation
   of such shares in the first place.

   https://github.com/cs3org/reva/pull/1051

*   Enhancement #1035: Refactor AppProvider workflow

   Simplified the app-provider configuration: storageID is worked out automatically and UIURL
   is suppressed for now. Implemented the new gRPC protocol from the gateway to the appprovider.

   https://github.com/cs3org/reva/pull/1035

*   Enhancement #1059: Improve timestamp precision while logging

   Previously, the timestamp associated with a log just had the hour and minute, which made
   debugging quite difficult. This PR increases the precision of the associated timestamp.

   https://github.com/cs3org/reva/pull/1059

*   Enhancement #1037: System information HTTP service

   This service exposes system information via an HTTP endpoint. This currently only includes
   Reva version information but can be extended easily. The information are exposed in the form of
   Prometheus metrics so that we can gather these in a streamlined way.

   https://github.com/cs3org/reva/pull/1037

*   Enhancement #995: Add UID and GID to the user object from user package

   Currently, the UID and GID for users need to be read from the local system which requires local
   users to be present. This change retrieves that information from the user and auth packages and
   adds methods to retrieve it.

   https://github.com/cs3org/reva/pull/995

Changelog for reva 1.0.0 (2020-07-28)
=======================================

The following sections list the changes in reva 1.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #941: Fix initialization of json share manager
*   Fix #1006: Check if SMTP credentials are nil
*   Chg #965: Remove protocol from partner domains to match gocdb config
*   Enh #986: Added signing key capability
*   Enh #922: Add tutorial for deploying WOPI and Reva locally
*   Enh #979: Skip changelog enforcement for bot PRs
*   Enh #965: Enforce adding changelog in make and CI
*   Enh #1016: Do not enforce changelog on release
*   Enh #969: Allow requests to hosts with unverified certificates
*   Enh #914: Make httpclient configurable
*   Enh #972: Added a site locations exporter to Mentix
*   Enh #1000: Forward share invites to the provider selected in meshdirectory
*   Enh #1002: Pass the link to the meshdirectory service in token mail
*   Enh #1008: Use proper logging for ldap auth requests
*   Enh #970: Add required headers to SMTP client to prevent being tagged as spam
*   Enh #996: Split LDAP user filters
*   Enh #1007: Update go-tus version
*   Enh #1004: Update github.com/go-ldap/ldap to v3
*   Enh #974: Add functionality to create webdav references for OCM shares

Details
-------

*   Bugfix #941: Fix initialization of json share manager

   When an empty shares.json file existed the json share manager would fail while trying to
   unmarshal the empty file.

   https://github.com/cs3org/reva/issues/941
   https://github.com/cs3org/reva/pull/940

*   Bugfix #1006: Check if SMTP credentials are nil

   Check if SMTP credentials are nil before passing them to the SMTPClient, causing it to crash.

   https://github.com/cs3org/reva/pull/1006

*   Change #965: Remove protocol from partner domains to match gocdb config

   Minor changes for OCM cross-partner testing.

   https://github.com/cs3org/reva/pull/965

*   Enhancement #986: Added signing key capability

   The ocs capabilities can now hold the boolean flag to indicate url signing endpoint and
   middleware are available

   https://github.com/cs3org/reva/pull/986

*   Enhancement #922: Add tutorial for deploying WOPI and Reva locally

   Add a new tutorial on how to run Reva and Wopiserver together locally

   https://github.com/cs3org/reva/pull/922

*   Enhancement #979: Skip changelog enforcement for bot PRs

   Skip changelog enforcement for bot PRs.

   https://github.com/cs3org/reva/pull/979

*   Enhancement #965: Enforce adding changelog in make and CI

   When adding a feature or fixing a bug, a changelog needs to be specified, failing which the build
   wouldn't pass.

   https://github.com/cs3org/reva/pull/965

*   Enhancement #1016: Do not enforce changelog on release

   While releasing a new version of Reva, make release was failing because it was enforcing a
   changelog entry.

   https://github.com/cs3org/reva/pull/1016

*   Enhancement #969: Allow requests to hosts with unverified certificates

   Allow OCM to send requests to other mesh providers with the option of skipping certificate
   verification.

   https://github.com/cs3org/reva/pull/969

*   Enhancement #914: Make httpclient configurable

   - Introduce Options for the httpclient (#914)

   https://github.com/cs3org/reva/pull/914

*   Enhancement #972: Added a site locations exporter to Mentix

   Mentix now offers an endpoint that exposes location information of all sites in the mesh. This
   can be used in Grafana's world map view to show the exact location of every site.

   https://github.com/cs3org/reva/pull/972

*   Enhancement #1000: Forward share invites to the provider selected in meshdirectory

   Added a share invite forward OCM endpoint to the provider links (generated when a user picks a
   target provider in the meshdirectory service web interface), together with an invitation
   token and originating provider domain passed to the service via query params.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1000

*   Enhancement #1002: Pass the link to the meshdirectory service in token mail

   Currently, we just forward the token and the original user's domain when forwarding an OCM
   invite token and expect the user to frame the forward invite URL. This PR instead passes the link
   to the meshdirectory service, from where the user can pick the provider they want to accept the
   invite with.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1002

*   Enhancement #1008: Use proper logging for ldap auth requests

   Instead of logging to stdout we now log using debug level logging or error level logging in case
   the configured system user cannot bind to LDAP.

   https://github.com/cs3org/reva/pull/1008

*   Enhancement #970: Add required headers to SMTP client to prevent being tagged as spam

   Mails being sent through the client, specially through unauthenticated SMTP were being
   tagged as spam due to missing headers.

   https://github.com/cs3org/reva/pull/970

*   Enhancement #996: Split LDAP user filters

   The current LDAP user and auth filters only allow a single `%s` to be replaced with the relevant
   string. The current `userfilter` is used to lookup a single user, search for share recipients
   and for login. To make each use case more flexible we split this in three and introduced
   templates.

   For the `userfilter` we moved to filter templates that can use the CS3 user id properties
   `{{.OpaqueId}}` and `{{.Idp}}`: ``` userfilter =
   "(&(objectclass=posixAccount)(|(ownclouduuid={{.OpaqueId}})(cn={{.OpaqueId}})))"
   ```

   We introduced a new `findfilter` that is used when searching for users. Use it like this: ```
   findfilter =
   "(&(objectclass=posixAccount)(|(cn={{query}}*)(displayname={{query}}*)(mail={{query}}*)))"
   ```

   Furthermore, we also introduced a dedicated login filter for the LDAP auth manager: ```
   loginfilter = "(&(objectclass=posixAccount)(|(cn={{login}})(mail={{login}})))" ```

   These filter changes are backward compatible: `findfilter` and `loginfilter` will be
   derived from the `userfilter` by replacing `%s` with `{{query}}` and `{{login}}`
   respectively. The `userfilter` replaces `%s` with `{{.OpaqueId}}`

   Finally, we changed the default attribute for the immutable uid of a user to
   `ms-DS-ConsistencyGuid`. See
   https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
   for the background. You can fall back to `objectguid` or even `samaccountname` but you will run
   into trouble when user names change. You have been warned.

   https://github.com/cs3org/reva/pull/996

*   Enhancement #1007: Update go-tus version

   The lib now uses go mod which should help golang to sort out dependencies when running `go mod
   tidy`.

   https://github.com/cs3org/reva/pull/1007

*   Enhancement #1004: Update github.com/go-ldap/ldap to v3

   In the current version of the ldap lib attribute comparisons are case sensitive. With v3
   `GetEqualFoldAttributeValue` is introduced, which allows a case insensitive comparison.
   Which AFAICT is what the spec says: see
   https://github.com/go-ldap/ldap/issues/129#issuecomment-333744641

   https://github.com/cs3org/reva/pull/1004

*   Enhancement #974: Add functionality to create webdav references for OCM shares

   Webdav references will now be created in users' shares directory with the target set to the
   original resource's location in their mesh provider.

   https://github.com/cs3org/reva/pull/974

Changelog for reva 0.1.0 (2020-03-18)
=======================================

The following sections list the changes in reva 0.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Enh #402: Build daily releases
*   Enh #416: Improve developer experience
*   Enh #468: remove vendor support
*   Enh #545: simplify configuration
*   Enh #561: improve the documentation
*   Enh #562: support home storages

Details
-------

*   Enhancement #402: Build daily releases

   Reva was not building releases of commits to the master branch. Thanks to @zazola.

   Commit-based released are generated every time a PR is merged into master. These releases are
   available at: https://reva-releases.web.cern.ch

   https://github.com/cs3org/reva/pull/402

*   Enhancement #416: Improve developer experience

   Reva provided the option to be run with a single configuration file by using the -c config flag.

   This PR adds the flag -dev-dir than can point to a directory containing multiple config files.
   The reva daemon will launch a new process per configuration file.

   Kudos to @refs.

   https://github.com/cs3org/reva/pull/416

*   Enhancement #468: remove vendor support

   Because @dependabot cannot update in a clean way the vendor dependecies Reva removed support
   for vendored dependencies inside the project.

   Dependencies will continue to be versioned but they will be downloaded when compiling the
   artefacts.

   https://github.com/cs3org/reva/pull/468
   https://github.com/cs3org/reva/pull/524

*   Enhancement #545: simplify configuration

   Reva configuration was difficul as many of the configuration parameters were not providing
   sane defaults. This PR and the related listed below simplify the configuration.

   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/536
   https://github.com/cs3org/reva/pull/568

*   Enhancement #561: improve the documentation

   Documentation has been improved and can be consulted here: https://reva.link

   https://github.com/cs3org/reva/pull/561
   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/568

*   Enhancement #562: support home storages

   Reva did not have any functionality to handle home storages. These PRs make that happen.

   https://github.com/cs3org/reva/pull/562
   https://github.com/cs3org/reva/pull/510
   https://github.com/cs3org/reva/pull/493
   https://github.com/cs3org/reva/pull/476
   https://github.com/cs3org/reva/pull/469
   https://github.com/cs3org/reva/pull/436
   https://github.com/cs3org/reva/pull/571

Changelog for reva 0.0.1 (2019-10-24)
=======================================

The following sections list the changes in reva 0.0.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Enh #334: Create release procedure for Reva

Details
-------

*   Enhancement #334: Create release procedure for Reva

   Reva did not have any procedure to release versions. This PR brings a new tool to release Reva
   versions (tools/release) and prepares the necessary files for artefact distributed made
   from Drone into Github pages.

   https://github.com/cs3org/reva/pull/334

