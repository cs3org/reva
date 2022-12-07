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
