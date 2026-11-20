Changelog for reva 3.15.3 (2026-11-20)
=======================================

The following sections list the changes in reva 3.15.3 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #6007: Dont get user groups when resolving users in sharemgr
 * Fix #6002: Don't fetch groups for federated or guest accounts
 * Fix #6030: Allow downloading shared files
 * Fix #6039: Update ref to publiccode file in release script
 * Fix #5751: Fix strconv.Atoi overflow on 32-bit platforms
 * Fix #5979: Don't use the default gowebdav auth
 * Fix #6006: Look up each project space once per listing
 * Chg #6004: New website
 * Chg #6031: OCM always signs and needs signing_key and endpoint
 * Chg #6021: Add publiccode.yaml
 * Chg #6037: Add creator of share to shallow job journal
 * Enh #6034: Cephmount allow byte ranges
 * Enh #5991: Expose labels over libregraph
 * Enh #6019: Log the user and user agent of each http request
 * Enh #6018: Count warn and error log messages
 * Enh #6016: Log the account type at login
 * Enh #5986: OCM cleanups
 * Enh #6009: Serve webapps to OCM users
 * Enh #6015: Count created projects in the projects stats
 * Enh #6008: Refuse propfind with depth infinity
 * Enh #6029: Report current batch in shallow job
 * Enh #6027: Return full public link url in GRPC layer
 * Enh #6000: Lookup grantee user type in the grpc gateway layer

Details
-------

 * Bugfix #6007: Dont get user groups when resolving users in sharemgr

   https://github.com/cs3org/reva/pull/6007

 * Bugfix #6002: Don't fetch groups for federated or guest accounts

   Public-link guests and OCM users are not known to any user backend, so every group lookup for
   them failed. With the REST driver, each lookup was a failed request to the CERN authorization
   service.

   https://github.com/cs3org/reva/pull/6002

 * Bugfix #6030: Allow downloading shared files

   We allow HEAD and GET on resource IDs now, as this is the call the web client makes to download a
   shared file.

   https://github.com/cs3org/reva/pull/6030

 * Bugfix #6039: Update ref to publiccode file in release script

   https://github.com/cs3org/reva/pull/6039

 * Bugfix #5751: Fix strconv.Atoi overflow on 32-bit platforms

   `strconv.Atoi` uses the platform `int` size, which is 32-bit on armv7. The version and trash
   suffixes are 13-digit millisecond timestamps, so parsing them overflowed and the entry was
   silently dropped from `ListRevisions` and the recycle bin. Parse them as `int64` instead.

   https://github.com/cs3org/reva/pull/5751

 * Bugfix #5979: Don't use the default gowebdav auth

   When using the default gowebdav authentication the first request is sent twice, to limit the
   number of requests this is now fixed.

   https://github.com/cs3org/reva/pull/5979

 * Bugfix #6006: Look up each project space once per listing

   For each entry of a listing, the gateway asked the spaces registry for the project space, and the
   registry stat'ed the home and all public spaces to find it. For a user who is not a project member
   nothing was cached. Space roles also gave back share permissions on a space root, so a project
   admin could share the root of the project.

   https://github.com/cs3org/reva/pull/6006

 * Change #6004: New website

   The website hosting the auto-generated docs has been completely renewed

   https://github.com/cs3org/reva/pull/6004/

 * Change #6031: OCM always signs and needs signing_key and endpoint

   We will now enforce that all our outgoing OCM requests are signed and this also means that a
   signing key and the public endpoint are required if OCM is enabled.

   https://github.com/cs3org/reva/pull/6031

 * Change #6021: Add publiccode.yaml

   https://github.com/cs3org/reva/pull/6021

 * Change #6037: Add creator of share to shallow job journal

   https://github.com/cs3org/reva/pull/6037

 * Enhancement #6034: Cephmount allow byte ranges

   When calling download on the Cephmount driver we previously did not allow byte ranges, now this
   is allowed and should be served since clients like WebDav rclone require this.

   https://github.com/cs3org/reva/pull/6034

 * Enhancement #5991: Expose labels over libregraph

   This way, we can support adding spaces and shares to your favorites

   https://github.com/cs3org/reva/pull/5991

 * Enhancement #6019: Log the user and user agent of each http request

   The "processed http request" line now carries the authenticated user and the request's user
   agent, so the usage metrics can count the users of each client, the web UI included, whose proxy
   cannot see the user.

   https://github.com/cs3org/reva/pull/6019

 * Enhancement #6018: Count warn and error log messages

   Expose `reva_log_messages_total{level}` on `/metrics` (prefix set by `logs.namespace`),
   incremented for each emitted log message at warn level or above.

   https://github.com/cs3org/reva/pull/6018

 * Enhancement #6016: Log the account type at login

   The gateway logs one "user authenticated" line per successful login, with the username, the
   account type and the auth type, for the usage metrics.

   https://github.com/cs3org/reva/pull/6016

 * Enhancement #5986: OCM cleanups

   This PR regroups some left-over cleanup on the OCM logic, following recent merges.

   https://github.com/cs3org/reva/pull/5986

 * Enhancement #6009: Serve webapps to OCM users

   https://github.com/cs3org/reva/pull/6009

 * Enhancement #6015: Count created projects in the projects stats

   The projects catalogue statistics now include a projects_created counter by storage, which
   also counts soft-deleted projects.

   https://github.com/cs3org/reva/pull/6015

 * Enhancement #6008: Refuse propfind with depth infinity

   A PROPFIND with Depth: infinity walked the whole tree in one request and kept all of it in memory.
   One such request on a project folder took more than two minutes. Web only uses depth 0 and 1, so
   ocdav now answers it with 403, as RFC 4918 allows, for files, spaces and the trash bin. Depth
   infinity on COPY and MOVE is not changed.

   https://github.com/cs3org/reva/pull/6008/

 * Enhancement #6029: Report current batch in shallow job

   https://github.com/cs3org/reva/pull/6029

 * Enhancement #6027: Return full public link url in GRPC layer

   The token was the only thing that was returned when creating a public link in the GRPC layer, as
   the full public link was constructed in the HTTP layer. This is fixed with this change as it
   causes issues for the GRPC clients.

   https://github.com/cs3org/reva/pull/6027

 * Enhancement #6000: Lookup grantee user type in the grpc gateway layer

   Previously this has been done in the HTTP layer, which means requests going straight to the GRPC
   layer will fail if there's no user type set

   https://github.com/cs3org/reva/pull/6000


