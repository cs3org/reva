Changelog for reva 2.22.0 (2024-07-29)
=======================================

The following sections list the changes in reva 2.22.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4741: Always find unique providers
*   Fix #4762: Blanks in dav Content-Disposition header
*   Fix #4775: Fixed the response code when copying the shared from to personal
*   Fix #4633: Allow all users to create internal links
*   Fix #4771: Deleting resources via their id
*   Fix #4768: Fixed the file name validation if nodeid is used
*   Fix #4758: Fix moving locked files, enable handling locked files via ocdav
*   Fix #4774: Fix micro ocdav service init and registration
*   Fix #4776: Fix response code for DEL file that in postprocessing
*   Fix #4746: Uploading the same file multiple times leads to orphaned blobs
*   Fix #4778: Zero byte uploads
*   Chg #4759: Updated to the latest version of the go-cs3apis
*   Chg #4773: Ocis bumped
*   Enh #4766: Set archiver output format via query parameter
*   Enh #4763: Improve posixfs storage driver

Details
-------

*   Bugfix #4741: Always find unique providers

   The gateway will now always try to find a single unique provider. It has stopped aggregating
   multiple ListContainer responses when we removed any business logic from it.

   https://github.com/cs3org/reva/pull/4741
   https://github.com/cs3org/reva/pull/4740
   https://github.com/cs3org/reva/pull/2394

*   Bugfix #4762: Blanks in dav Content-Disposition header

   We've fixed the encoding of blanks in the dav `Content-Disposition` header.

   https://github.com/owncloud/web/issues/11169
   https://github.com/cs3org/reva/pull/4762

*   Bugfix #4775: Fixed the response code when copying the shared from to personal

   We fixed the response code when copying the file from shares to personal space with a secure view
   role.

   https://github.com/owncloud/ocis/issues/9482
   https://github.com/cs3org/reva/pull/4775

*   Bugfix #4633: Allow all users to create internal links

   Due to a bug, not all space members were allowed to create internal links. This has been fixed.

   https://github.com/owncloud/ocis/issues/8960
   https://github.com/cs3org/reva/pull/4633

*   Bugfix #4771: Deleting resources via their id

   We fixed a bug where deleting resources by using their id via the `/dav/spaces/` endpoint would
   not work.

   https://github.com/owncloud/ocis/issues/9619
   https://github.com/cs3org/reva/pull/4771

*   Bugfix #4768: Fixed the file name validation if nodeid is used

   We have fixed the file name validation if nodeid is used

   https://github.com/owncloud/ocis/issues/9568
   https://github.com/cs3org/reva/pull/4768

*   Bugfix #4758: Fix moving locked files, enable handling locked files via ocdav

   We fixed a problem when trying to move locked files. We also enabled the ocdav service to handle
   locked files.

   https://github.com/cs3org/reva/pull/4758

*   Bugfix #4774: Fix micro ocdav service init and registration

   We no longer call Init to configure default options because it was replacing the existing
   options.

   https://github.com/cs3org/reva/pull/4774

*   Bugfix #4776: Fix response code for DEL file that in postprocessing

   We fixed the response code when DELETE and MOVE requests to the file that is still in
   post-processing.

   https://github.com/owncloud/ocis/issues/9432
   https://github.com/cs3org/reva/pull/4776

*   Bugfix #4746: Uploading the same file multiple times leads to orphaned blobs

   Fixed a bug where multiple uploads of the same file would lead to orphaned blobs in the
   blobstore. These orphaned blobs will now be deleted.

   https://github.com/cs3org/reva/pull/4746

*   Bugfix #4778: Zero byte uploads

   Zero byte uploads would trigger postprocessing which lead to breaking pipelines.

   https://github.com/cs3org/reva/pull/4778

*   Change #4759: Updated to the latest version of the go-cs3apis

   The go-cs3apis dependency was updated to the latest version

   https://github.com/owncloud/ocis/issues/9554
   https://github.com/cs3org/reva/pull/4759

*   Change #4773: Ocis bumped

   Ocis bumped. The expected failures removed.

   https://github.com/cs3org/reva/pull/4773

*   Enhancement #4766: Set archiver output format via query parameter

   Sets the archive output format e.G "tar" via the url query parameter "output-format",
   possible params are "zip" and "tar", falls back to "zip".

   https://github.com/owncloud/ocis/issues/9399
   https://github.com/owncloud/web/issues/11080
   https://github.com/cs3org/reva/pull/4766

*   Enhancement #4763: Improve posixfs storage driver

   Improve the posixfs storage driver by fixing several issues and corner cases.

   https://github.com/cs3org/reva/pull/4763

