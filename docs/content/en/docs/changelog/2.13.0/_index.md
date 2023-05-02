
---
title: "v2.13.0"
linkTitle: "v2.13.0"
weight: 40
description: >
  Changelog for Reva v2.13.0 (2023-05-02)
---

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

