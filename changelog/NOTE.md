Changelog for reva 2.26.7 (2024-12-04)
=======================================

The following sections list the changes in reva 2.26.7 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4983: Delete stale shares in the jsoncs3 share manager
*   Fix #4963: Fix name and displayName in an ocm
*   Fix #4968: Jsoncs3 cache fixes
*   Fix #4928: Propagate lock in PROPPATCH
*   Fix #4971: Use manager to list shares
*   Enh #4978: We added more trace spans in decomposedfs
*   Enh #4921: Polish propagation related code

Details
-------

*   Bugfix #4983: Delete stale shares in the jsoncs3 share manager

   The jsoncs3 share manager now properly deletes all references to removed shares and shares
   that belong to a space that was deleted

   https://github.com/cs3org/reva/pull/4983
   https://github.com/cs3org/reva/pull/4975

*   Bugfix #4963: Fix name and displayName in an ocm

   Fixed name and displayName in an ocm PROPFIND response

   https://github.com/owncloud/ocis/issues/10582
   https://github.com/cs3org/reva/pull/4963

*   Bugfix #4968: Jsoncs3 cache fixes

   The jsoncs3 share manager now retries persisting if the file already existed and picks up the
   etag of the upload response in all cases.

   https://github.com/cs3org/reva/pull/4968
   https://github.com/cs3org/reva/pull/4532

*   Bugfix #4928: Propagate lock in PROPPATCH

   Clients using locking (ie. Windows) could not create/copy files over webdav as file seemed to
   be locked.

   https://github.com/cs3org/reva/pull/4928

*   Bugfix #4971: Use manager to list shares

   When updating a received share the usershareprovider now uses the share manager directly to
   list received shares instead of going through the gateway again.

   https://github.com/cs3org/reva/pull/4971

*   Enhancement #4978: We added more trace spans in decomposedfs

   https://github.com/cs3org/reva/pull/4978

*   Enhancement #4921: Polish propagation related code

   We polished some corner cases for propagation that reduce log messages in normal operation.

   https://github.com/cs3org/reva/pull/4921

