Changelog for reva 2.7.2 (2022-07-18)
=======================================

The following sections list the changes in reva 2.7.2 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3079: Allow empty permissions
 * Fix #3084: Spaces relatated permissions and providerid cleanup
 * Fix #3083: Add space id to ItemTrashed event

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
