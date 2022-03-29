
---
title: "v2.1.0"
linkTitle: "v2.1.0"
weight: 40
description: >
  Changelog for Reva v2.1.0 (2022-03-29)
---

Changelog for reva 2.1.0 (2022-03-29)
=======================================

The following sections list the changes in reva 2.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2636: Delay reconnect log for events
 * Fix #2645: Avoid warning about missing .flock files
 * Fix #2625: Fix locking on publik links and the decomposed filesystem
 * Fix #2643: Emit linkaccessfailed event when share is nil
 * Fix #2646: Replace public mountpoint fileid with grant fileid in ocdav
 * Fix #2612: Adjust the scope handling to support the spaces architecture
 * Fix #2621: Send events only if response code is `OK`
 * Chg #2574: Switch NATS backend
 * Chg #2667: Allow LDAP groups to have no gidNumber
 * Chg #3233: Improve quota handling
 * Chg #2600: Use the cs3 share api to manage spaces
 * Enh #2644: Add new public share manager
 * Enh #2626: Add new share manager
 * Enh #2624: Add etags to virtual spaces
 * Enh #2639: File Events
 * Enh #2627: Add events for sharing action
 * Enh #2664: Add grantID to mountpoint
 * Enh #2622: Allow listing shares in spaces via the OCS API
 * Enh #2623: Add space aliases
 * Enh #2647: Add space specific events
 * Enh #3345: Add the spaceid to propfind responses
 * Enh #2616: Add etag to spaces response
 * Enh #2628: Add spaces aware trash-bin API

Details
-------

 * Bugfix #2636: Delay reconnect log for events

   Print reconnect information log only when reconnect time is bigger than a second

   https://github.com/cs3org/reva/pull/2636

 * Bugfix #2645: Avoid warning about missing .flock files

   These flock files appear randomly because of file locking. We can savely ignore them.

   https://github.com/cs3org/reva/pull/2645

 * Bugfix #2625: Fix locking on publik links and the decomposed filesystem

   We've fixed the behavior of locking on the decomposed filesystem, so that now app based locks
   can be modified user independently (needed for WOPI integration). Also we added a check, if a
   lock is already expired and if so, we lazily delete the lock. The InitiateUploadRequest now
   adds the Lock to the upload metadata so that an upload to an locked file is possible.

   We'v added the locking api requests to the public link scope checks, so that locking also can be
   used on public links (needed for WOPI integration).

   https://github.com/cs3org/reva/pull/2625

 * Bugfix #2643: Emit linkaccessfailed event when share is nil

   The code no longer panics when a link access failed event has no share.

   https://github.com/cs3org/reva/pull/2643

 * Bugfix #2646: Replace public mountpoint fileid with grant fileid in ocdav

   We now show the same resoucre id for resources when accessing them via a public links as when
   using a logged in user. This allows the web ui to start a WOPI session with the correct resource
   id.

   https://github.com/cs3org/reva/issues/2635
   https://github.com/cs3org/reva/pull/2646

 * Bugfix #2612: Adjust the scope handling to support the spaces architecture

   The scope authentication interceptors weren't updated to the spaces architecture and
   couldn't authenticate some requests.

   https://github.com/cs3org/reva/pull/2612

 * Bugfix #2621: Send events only if response code is `OK`

   Before events middleware was sending events also when the resulting status code was not `OK`.
   This is clearly a bug.

   https://github.com/cs3org/reva/pull/2621

 * Change #2574: Switch NATS backend

   We've switched the NATS backend from Streaming to JetStream, since NATS Streaming is
   depreciated.

   https://github.com/cs3org/reva/pull/2574

 * Change #2667: Allow LDAP groups to have no gidNumber

   Similar to the user-provider allow a group to have no gidNumber. Assign a default in that case.

   https://github.com/cs3org/reva/pull/2667

 * Change #3233: Improve quota handling

   GetQuota now returns 0 when no quota was set instead of the disk size. Also added a new return
   value for the remaining space which will either be quota - used bytes or if no quota was set the
   free disk size.

   https://github.com/owncloud/ocis/issues/3233
   https://github.com/cs3org/reva/pull/2666
   https://github.com/cs3org/reva/pull/2688

 * Change #2600: Use the cs3 share api to manage spaces

   We now use the cs3 share Api to manage the space roles. We do not send the request to the share
   manager, the permissions are stored in the storage provider

   https://github.com/cs3org/reva/pull/2600
   https://github.com/cs3org/reva/pull/2620
   https://github.com/cs3org/reva/pull/2687

 * Enhancement #2644: Add new public share manager

   We added a new public share manager which uses the new metadata storage backend for persisting
   the public share information.

   https://github.com/cs3org/reva/pull/2644

 * Enhancement #2626: Add new share manager

   We added a new share manager which uses the new metadata storage backend for persisting the
   share information.

   https://github.com/cs3org/reva/pull/2626

 * Enhancement #2624: Add etags to virtual spaces

   The shares storage provider didn't include the etag in virtual spaces like the shares jail or
   mountpoints.

   https://github.com/cs3org/reva/pull/2624

 * Enhancement #2639: File Events

   Adds file based events. See `pkg/events/files.go` for full list

   https://github.com/cs3org/reva/pull/2639

 * Enhancement #2627: Add events for sharing action

   Includes lifecycle events for shares and public links doesn't include federated sharing
   events for now see full list of events in `pkg/events/types.go`

   https://github.com/cs3org/reva/pull/2627

 * Enhancement #2664: Add grantID to mountpoint

   We distinguish between the mountpoint of a share and the grant where the original file is
   located on the storage.

   https://github.com/cs3org/reva/pull/2664

 * Enhancement #2622: Allow listing shares in spaces via the OCS API

   Added a `space_ref` parameter to the list shares endpoints so that one can list shares inside of
   spaces.

   https://github.com/cs3org/reva/pull/2622

 * Enhancement #2623: Add space aliases

   Space aliases can be used to resolve spaceIDs in a client.

   https://github.com/cs3org/reva/pull/2623

 * Enhancement #2647: Add space specific events

   See `pkg/events/spaces.go` for full list

   https://github.com/cs3org/reva/pull/2647

 * Enhancement #3345: Add the spaceid to propfind responses

   Added the spaceid to propfind responses so that clients have the necessary data to send
   subsequent requests.

   https://github.com/owncloud/ocis/issues/3345
   https://github.com/cs3org/reva/pull/2657

 * Enhancement #2616: Add etag to spaces response

   Added the spaces etag to the response when listing spaces.

   https://github.com/cs3org/reva/pull/2616

 * Enhancement #2628: Add spaces aware trash-bin API

   Added the webdav trash-bin endpoint for spaces.

   https://github.com/cs3org/reva/pull/2628


