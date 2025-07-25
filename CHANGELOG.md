Changelog for reva 3.0.1 (2025-07-04)
=======================================

The following sections list the changes in reva 3.0.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5213: Home creation
 * Fix #5190: List file versions
 * Fix #5189: Shares parent reference
 * Fix #5204: Restore trashbin
 * Fix #5219: Reva v3
 * Fix #5198: Let sharees list versions
 * Fix #5216: Download / restore versions in spaces
 * Enh #5220: Clean up obosolete OCIS tests
 * Enh #4864: Add HTTP header to disable versioning on EOS
 * Enh #5201: EOS gRPC cleanup
 * Enh #4883: Use newfind command in EOS
 * Enh #5211: Add error code to DAV responses
 * Enh #5210: Libregraph permission support
 * Enh #5205: Ignore unknown routes
 * Enh #5197: Pprof improvements
 * Enh #5217: Spaces improvements

Details
-------

 * Bugfix #5213: Home creation

   https://github.com/cs3org/reva/pull/5213

 * Bugfix #5190: List file versions

   - moved versions-related functions to utils package - new `spaceHref` function for listing
   file versions - adapts code from #2855 for restoring and downloading file versions - add parent
   info to propfind response - add space info to parent reference

   https://github.com/cs3org/reva/pull/5190

 * Bugfix #5189: Shares parent reference

   - change: replace `md.Id.SpaceID` with `<storage-id>?<space-id>` - fix: parentReference -
   add space info to id - removes double encoding of driveId - new function to return relative path
   inside a space root - refactor space utils: - reorder functions (Encode > Decode > Parse) -
   returns `SpaceID` instead of `path` in `DecodeResourceID` - new comments

   https://github.com/cs3org/reva/pull/5189

 * Bugfix #5204: Restore trashbin

   https://github.com/cs3org/reva/pull/5204

 * Bugfix #5219: Reva v3

   Made reva module v3, to align with the github release

   https://github.com/cs3org/reva/pull/5219

 * Bugfix #5198: Let sharees list versions

   https://github.com/cs3org/reva/pull/5198/

 * Bugfix #5216: Download / restore versions in spaces

   * Some extra logging * Fixed a bug in IsVersionFolder * Fixed a bug in handling GET requests on the
   VersionsHandler

   https://github.com/cs3org/reva/pull/5216

 * Enhancement #5220: Clean up obosolete OCIS tests

   https://github.com/cs3org/reva/pull/5220

 * Enhancement #4864: Add HTTP header to disable versioning on EOS

   https://github.com/cs3org/reva/pull/4864
   This
   enhancement
   introduces
   a
   new
   header,
   %60X-Disable-Versioning%60,
   on
   PUT
   requests.
   EOS
   will
   not
   version
   this
   file
   save
   whenever
   this
   header
   is
   set
   with
   a
   truthy
   value.
   See
   also:

 * Enhancement #5201: EOS gRPC cleanup

   Remove reliance on binary client for some operations, split up EOS gRPC driver into several
   files

   https://github.com/cs3org/reva/pull/5201

 * Enhancement #4883: Use newfind command in EOS

   The EOS binary storage driver was still using EOS's oldfind command, which is deprecated. We
   now moved to the new find command, for which an extra flag (--skip-version-dirs) is needed.

   https://github.com/cs3org/reva/pull/4883

 * Enhancement #5211: Add error code to DAV responses

   - code adpated from the edge branch (#4749 and #4653) - new `errorCode` parameter in `Marshal`
   function

   https://github.com/cs3org/reva/pull/5211

 * Enhancement #5210: Libregraph permission support

   Extension of the libregraph API to fix the following issues: * Creating links / shares now gets a
   proper response * Support for updating links / shares * Support for deleting links / shares *
   Removal of unsupported roles from /roleDefinitions endpoint

   https://github.com/cs3org/reva/pull/5210

 * Enhancement #5205: Ignore unknown routes

   Currently, the gateway crashes with a fatal error if it encounters any unknown routes in the
   routing table. Instead, we log the error and ignore the routes, which should make upgrades in
   the routing table easier.

   https://github.com/cs3org/reva/pull/5205

 * Enhancement #5197: Pprof improvements

   https://github.com/cs3org/reva/pull/5197

 * Enhancement #5217: Spaces improvements

   Extended libregraph API, fixed restoring / downloading revisions in spaces

   https://github.com/cs3org/reva/pull/5217


Changelog for reva 3.0.0 (2025-06-03)
=======================================

The following sections list the changes in reva 3.0.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5082: Wrong quota total reported
 * Fix #5120: Allow folders to be un-favorited
 * Fix #5123: Use binary client for Attrs
 * Fix #5119: Stop sending non-UTF8 strings over gRPC
 * Fix #5124: Apps: fixed UserAgent matching
 * Fix #5122: Use the correct eos app header
 * Fix #5143: Eosgrpc: fixed panic with ACLs handling
 * Fix #5156: Handlenew failed to handle spaces id
 * Fix #5150: GetSpace with spaceID from URL
 * Fix #5153: Ocdav/put response compatible with spaces
 * Fix #5133: Fix broken handling of range requests
 * Fix #5149: ListMyOfficeFiles
 * Fix #5148: ListMyOfficeFiles
 * Fix #5064: Impersonate owner on ListRevisions
 * Fix #5166: Check for spaces in listshares
 * Fix #5084: Use logicalbytes instead of bytes
 * Fix #5044: Return an error when EOS List errors
 * Fix #5072: Impersonate owner on Revisions
 * Fix #5151: Spaces + apps broken
 * Fix #5142: Renamed mock APIs
 * Fix #5152: Revisions in spaces
 * Chg #5155: ListMyOfficeFiles only lists projects, not home
 * Chg #5105: Move publicshare sql driver
 * Enh #5113: Log simplified user agent in apps
 * Enh #5107: Add file extension in the returning app URL log message
 * Enh #5085: Extend app /notify endpoint to allow reporting errors
 * Enh #5110: Log acl payload on AddACL on EOS over gRPC
 * Enh #5186: Clean up EOS driver
 * Enh #5132: Add expiration dates reinforcement
 * Enh #5158: Add expire date to capabilities
 * Enh #5080: Log viewmode in the returning app URL message
 * Enh #5160: Upgrade to go1.24
 * Enh #4940: Refactoring of /home
 * Enh #5174: Upgrade jwt library
 * Enh #5134: Libregraph API expansion
 * Enh #5127: ListMyOfficeFiles
 * Enh #5129: Add makefile option for local plugins
 * Enh #5076: Implement OCM 1.2
 * Enh #5029: Pseudo-transactionalize sharing
 * Enh #5109: Extra logging in publicshares
 * Enh #5167: Improved logging of rhttp router
 * Enh #4404: Add support for Spaces

Details
-------

 * Bugfix #5082: Wrong quota total reported

   The EOS `QuotaInfo` struct had fields for `AvailableBytes` and `AvailableInodes`, but these
   were used to mean the total. This is fixed now.

   https://github.com/cs3org/reva/pull/5082

 * Bugfix #5120: Allow folders to be un-favorited

   Currently, removing a folder from your favorites is broken, if you are the only one who has
   favorited it. This is because the UnsetAttr call to EOS over gRPC is failing. As a temporary
   workaround, we now always set it to empty.

   https://github.com/cs3org/reva/pull/5120

 * Bugfix #5123: Use binary client for Attrs

   EOS < 5.3 has a couple of bugs related to attributes: * Attributes can only be removed as root or
   the owner, but over gRPC we cannot become root * The recursive property is ignored on set
   attributes

   For these two issues, we circumvent them by calling the binary client until we have deployed EOS
   5.3

   https://github.com/cs3org/reva/pull/5123

 * Bugfix #5119: Stop sending non-UTF8 strings over gRPC

   EOS supports having non-UTF8 attributes, which get returned in a Stat. This is problematic for
   us, as we pass these attributes in the ArbitraryMetadata, which gets sent over gRPC. However,
   the protobuf language specification states:

   > A string must always contain UTF-8 encoded or 7-bit ASCII text, and cannot be longer than 2^32.

   An example of such an attribute is:

   
   User.$KERNEL.PURGE.SEC.FILEHASH="S��ϫ]���z��#1}��uU�v��8�L0R�9j�j��e?�2K�T<sJ�*�l���Dǭ��_[�>η�...��w�w[��Yg"

   We fix this by stripping non-UTF8 metadata entries before sending the ResourceInfo over gRPC

   https://github.com/cs3org/reva/pull/5119

 * Bugfix #5124: Apps: fixed UserAgent matching

   https://github.com/cs3org/reva/pull/5124

 * Bugfix #5122: Use the correct eos app header

   https://github.com/cs3org/reva/pull/5122

 * Bugfix #5143: Eosgrpc: fixed panic with ACLs handling

   Fixes a panic that happens when listing a folder where files have no SysACLs, parent == nil and
   versionFolder has ACLs.

   https://github.com/cs3org/reva/pull/5143

 * Bugfix #5156: Handlenew failed to handle spaces id

   HandleNew, which creates new office files etc., tried to parse the parent container ref via
   spaces, and had no fallback for non-spaces. This is now fixed.

   https://github.com/cs3org/reva/pull/5156

 * Bugfix #5150: GetSpace with spaceID from URL

   `getSpace` was getting the first element of path as the ID. This fix adds a new function
   `GetIdFromPath`, which uses the last element of the path as the ID.

   https://github.com/cs3org/reva/pull/5150

 * Bugfix #5153: Ocdav/put response compatible with spaces

   Checks if `spaces` is enabled before returning the `fileid` after a PUT request.

   https://github.com/cs3org/reva/pull/5153

 * Bugfix #5133: Fix broken handling of range requests

   Currently, the video preview in public links is broken, because the browser sends a "Range:
   bytes=0-" request. Since EOS over gRPC returns a ReadCloser on the file, which is not seekable,
   Reva currently returns a 416 RequestedRangeNotSatisfiable response, breaking the video
   preview.

   This PR modifies this behaviour to ignore the Range request in such cases.

   Additionally, some errors where removed. For example, when the request does not contain
   bytes=, Reva currently returns an error. However, RFC 7233 states:

   > An origin server MUST ignore a Range header field that contains a range unit it does not
   understand

   Thus, we now ignore these requests instead of returning a 416.

   https://github.com/cs3org/reva/pull/5133

 * Bugfix #5149: ListMyOfficeFiles

   Regex was simplified, and a cache was created to keep version folder fileinfo, to make sure we
   don't need a stat for every result

   https://github.com/cs3org/reva/pull/5149

 * Bugfix #5148: ListMyOfficeFiles

   There was a bug in the regex for excel files: "(.*?)(.xls|.XLS|)[x|X]?$" contains an extra "|"

   https://github.com/cs3org/reva/pull/5148

 * Bugfix #5064: Impersonate owner on ListRevisions

   ListRevisions is currently broken for projects, because this happens on behalf of the user,
   instead of the owner of the file. This behaviour is changed to do the call on behalf of the owner
   (if we are in a non-home space).

   https://github.com/cs3org/reva/pull/5064

 * Bugfix #5166: Check for spaces in listshares

   Opening files in their app from the "Shared with me" view was broken, because we returned spaces
   ids even on old clients.

   https://github.com/cs3org/reva/pull/5166

 * Bugfix #5084: Use logicalbytes instead of bytes

   EOS gRPC used `usedbytes` instead of `usedlogicalbytes` for calculating quota, resulting in
   a wrong view

   https://github.com/cs3org/reva/pull/5084

 * Bugfix #5044: Return an error when EOS List errors

   If we get an error while reading items, we now return the error to the user and break off the List
   operation We do not want to return a partial list, because then a sync client may delete local
   files that are missing on the server

   https://github.com/cs3org/reva/pull/5044

 * Bugfix #5072: Impersonate owner on Revisions

   The current implementation of Download / Restore Revisions is not consistent with
   ListRevisions, where we impersonate the owner in projects. We now also impersonate in the case
   of Download and Restore.

   https://github.com/cs3org/reva/pull/5072

 * Bugfix #5151: Spaces + apps broken

   There were still some places where Reva assumed that we are running spaces; while not verifying
   this. This caused non-spaces WebUIs to break. This is fixed now.

   https://github.com/cs3org/reva/pull/5151

 * Bugfix #5142: Renamed mock APIs

   The once called "mock" space APIs have been renamed as they provide static content to the web
   frontend

   https://github.com/cs3org/reva/pull/5142

 * Bugfix #5152: Revisions in spaces

   The PR that merged spaces compatibility broke listing revisions. This is now fixed.

   https://github.com/cs3org/reva/pull/5152

 * Change #5155: ListMyOfficeFiles only lists projects, not home

   ListMyOfficeFiles now shows only one project or the user's home, not both at the same time

   https://github.com/cs3org/reva/pull/5155

 * Change #5105: Move publicshare sql driver

   The publicshare sql driver has been moved to reva-plugins, to be consistent with the usershare
   sql driver.

   https://github.com/cs3org/reva/pull/5105

 * Enhancement #5113: Log simplified user agent in apps

   https://github.com/cs3org/reva/pull/5113

 * Enhancement #5107: Add file extension in the returning app URL log message

   https://github.com/cs3org/reva/pull/5107

 * Enhancement #5085: Extend app /notify endpoint to allow reporting errors

   https://github.com/cs3org/reva/pull/5085

 * Enhancement #5110: Log acl payload on AddACL on EOS over gRPC

   https://github.com/cs3org/reva/pull/5110

 * Enhancement #5186: Clean up EOS driver

   * removed unused eos drivers (home, grpc, grpchome) * removed dependency on wrapper

   https://github.com/cs3org/reva/pull/5186

 * Enhancement #5132: Add expiration dates reinforcement

   Public links of folders with RW permissions now have a configurable expiration date.

   https://github.com/cs3org/reva/pull/5132

 * Enhancement #5158: Add expire date to capabilities

   https://github.com/cs3org/reva/pull/5158

 * Enhancement #5080: Log viewmode in the returning app URL message

   https://github.com/cs3org/reva/pull/5080

 * Enhancement #5160: Upgrade to go1.24

   Upgrade to go1.24

   https://github.com/cs3org/reva/pull/5160

 * Enhancement #4940: Refactoring of /home

   https://github.com/cs3org/reva/pull/4940

 * Enhancement #5174: Upgrade jwt library

   Update the golang-jwt library to v5

   https://github.com/cs3org/reva/pull/5174

 * Enhancement #5134: Libregraph API expansion

   Several new libregraph API endpoints have been added in ocgraph. These endpoints are used by
   the updated front-end. Concretely, endpoints have been added for * searching users *
   searching groups * creating shares * creating public links

   https://github.com/cs3org/reva/pull/5134

 * Enhancement #5127: ListMyOfficeFiles

   This PR implements the temporary ListMyOfficeFiles functionality

   https://github.com/cs3org/reva/pull/5127

 * Enhancement #5129: Add makefile option for local plugins

   Added a new Make target that takes a local copy of reva-plugins

   https://github.com/cs3org/reva/pull/5129

 * Enhancement #5076: Implement OCM 1.2

   This PR brings in the implementation of parts of OpenCloudMesh 1.2, including: * Adopting the
   new properties of the OCM 1.2 payloads, without implementing any new functionality for now. In
   particular, any non-empty `requirement` in a share will be rejected (a test was added for
   that). * Extending the OCM discovery endpoint. * Using the remote OCM discovery endpoint to
   establish the full URL of an incoming remote share, regardless if provided or not. When sending
   a share, though, we still send a full URL. * Caching the webdav client used to connect to remote
   endpoints, with added compatibility to OCM 1.0 remote servers. * Some refactoring and
   consolidation of duplicated code. * Improved logging.

   https://github.com/cs3org/reva/pull/5076

 * Enhancement #5029: Pseudo-transactionalize sharing

   Currently, sharing is not transactionalized: setting ACLs and writing the share to the db is
   completely independent. In the current situation, shares are written to the db before setting
   the ACL, letting users falsely believe that they successfully shared a resource, even if
   setting the ACL afterwards fails. his enhancement improves the situation by doing the least
   reliable (setting ACLs on EOS) first: a) first pinging the db b) writing the ACLs c) writing to
   the db

   https://github.com/cs3org/reva/pull/5029

 * Enhancement #5109: Extra logging in publicshares

   Some extra log lines in the public-files DAV handler have been added

   https://github.com/cs3org/reva/pull/5109

 * Enhancement #5167: Improved logging of rhttp router

   https://github.com/cs3org/reva/pull/5167

 * Enhancement #4404: Add support for Spaces

   Credits to @gmgigi96

   https://github.com/cs3org/reva/pull/4404


Changelog for reva 1.29.0 (2025-01-07)
=======================================

The following sections list the changes in reva 1.29.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4898: Make ACL operations work over gRPC
 * Fix #4667: Fixed permission mapping to EOS ACLs
 * Fix #4520: Do not use version folders for xattrs in EOS
 * Fix #4599: Auth: increase verbosity of oidc parsing errors
 * Fix #5006: Blocking reva on listSharedWithMe
 * Fix #4557: Fix ceph build
 * Fix #5017: No empty favs attr
 * Fix #4620: Fix ulimits for EOS container deployment
 * Fix #5015: Fixed error reporting in the EOS gRPC client
 * Fix #4931: Fixed tree metadata following fix in EOS
 * Fix #4930: Make removal of favourites work
 * Fix #4574: Fix notifications
 * Fix #4790: Ocm: fixed domain not having a protocol scheme
 * Fix #4849: Drop assumptions about user types when dealing with shares
 * Fix #4894: No certs in EOS HTTP client
 * Fix #4810: Simplified error handling
 * Fix #4973: Handle parsing of favs over gRPC
 * Fix #4901: Broken PROPFIND perms on gRPC
 * Fix #4907: Public links: return error when owner could not be resolved
 * Fix #4591: Eos: fixed error reporting for too large recycle bin listing
 * Fix #4896: Fix nilpointer error in RollbackToVersion
 * Fix #4905: PurgeDate in ListDeletedEntries was ignored
 * Fix #4939: Revert 'make home layout configurable'
 * Enh #5028: Handle empty EOS traces
 * Enh #4911: Cephfs refactoring + make home layout configurable
 * Enh #4937: @labkode steps down as project owner
 * Enh #4579: Remove domain-specific code to other repos
 * Enh #4824: Refactor Ceph code
 * Enh #4797: Refactor CI jobs and bump to latest deps
 * Enh #4934: Access to EOS via tokens over gRPC
 * Enh #4870: Only load X509 on https
 * Enh #5014: Log app when creating EOS gRPC requests
 * Enh #4892: Do not read eos user ACLs any longer
 * Enh #4720: Differentiate quota for user types in EOS
 * Enh #4863: Favourites for eos/grpc
 * Enh #5013: Updated dependencies + moved to go 1.22
 * Enh #4514: Pass lock holder metadata on uploads
 * Enh #4970: Improved logging on createHome
 * Enh #4984: Drop shadow namespaces
 * Enh #4670: Ocm: support bearer token access
 * Enh #4977: Do not use root on EOS

Details
-------

 * Bugfix #4898: Make ACL operations work over gRPC

   This change solves two issues: * AddACL would fail, because the current implementation of
   AddACL in the EOS gRPC client always sets msg.Recursive = true. This causes issues on the EOS
   side, because it will try running a recursive find on a file, which fails. * RemoveACL would
   fail, because it tried matching ACL rules with a uid to ACL rules with a username. This PR changes
   this approach to use an approach similar to what is used in the binary client: just set the rule
   that you want to have deleted with no permissions.

   https://github.com/cs3org/reva/pull/4898

 * Bugfix #4667: Fixed permission mapping to EOS ACLs

   This is to remove "m" and "q" flags in EOS ACLs for regular write shares (no re-sharing).

   https://github.com/cs3org/reva/pull/4667

 * Bugfix #4520: Do not use version folders for xattrs in EOS

   This was a workaround needed some time ago. We revert now to the standard behavior, xattrs are
   stored on the files.

   https://github.com/cs3org/reva/pull/4520

 * Bugfix #4599: Auth: increase verbosity of oidc parsing errors

   This is to help further debugging of auth issues. An unrelated error reporting was also fixed.

   https://github.com/cs3org/reva/pull/4599

 * Bugfix #5006: Blocking reva on listSharedWithMe

   `listSharesWithMe` blocked a reva thread in the case that one of the shares was not resolvable.
   This has now been fixed

   https://github.com/cs3org/reva/pull/5006

 * Bugfix #4557: Fix ceph build

   https://github.com/cs3org/reva/pull/4557

 * Bugfix #5017: No empty favs attr

   See issue #5016: we now unset the favs attr if no more favs are set

   https://github.com/cs3org/reva/pull/5017

 * Bugfix #4620: Fix ulimits for EOS container deployment

   https://github.com/cs3org/reva/pull/4620

 * Bugfix #5015: Fixed error reporting in the EOS gRPC client

   This in particular fixes the lock-related errors

   https://github.com/cs3org/reva/pull/5015

 * Bugfix #4931: Fixed tree metadata following fix in EOS

   The treecount is now populated from the EOS response.

   https://github.com/cs3org/reva/pull/4931

 * Bugfix #4930: Make removal of favourites work

   Currently, removing a folder from your favourites is broken, because the handleFavAttr
   method is only called in SetAttr, not in UnsetAttr. This change fixes this.

   https://github.com/cs3org/reva/pull/4930

 * Bugfix #4574: Fix notifications

   https://github.com/cs3org/reva/pull/4574

 * Bugfix #4790: Ocm: fixed domain not having a protocol scheme

   This PR fixes a bug in the OCM open driver that causes it to be unable to probe OCM services at the
   remote server due to the domain having an unsupported protocol scheme. in this case domain
   doesn't have a scheme and the changes in this PR add a scheme to the domain before doing the probe.

   https://github.com/cs3org/reva/pull/4790

 * Bugfix #4849: Drop assumptions about user types when dealing with shares

   We may have external accounts with regular usernames (and with null uid), therefore the
   current logic to heuristically infer the user type from a grantee's username is broken. This PR
   removes those heuristics and requires the upper level to resolve the user type.

   https://github.com/cs3org/reva/pull/4849

 * Bugfix #4894: No certs in EOS HTTP client

   Omit HTTPS cert in EOS HTTP Client, as this causes authentication issues on EOS < 5.2.28. When
   EOS receives a certificate, it will look for this cert in the gridmap file. If it is not found
   there, the whole authn flow is aborted and the user is mapped to nobody.

   https://github.com/cs3org/reva/pull/4894

 * Bugfix #4810: Simplified error handling

   Minor rewording and simplification, following cs3org/OCM-API#90 and cs3org/OCM-API#91

   https://github.com/cs3org/reva/pull/4810

 * Bugfix #4973: Handle parsing of favs over gRPC

   To store user favorites, the key `user.http://owncloud.org/ns/favorite` maps to a list of
   users, in the format `u:username=1`. Right now, extracting the "correct" user doesn't happen
   in gRPC, while it is implemented in the EOS binary client. This feature has now been moved to the
   higher-level call in eosfs.

   https://github.com/cs3org/reva/pull/4973

 * Bugfix #4901: Broken PROPFIND perms on gRPC

   When using the EOS gRPC stack, the permissions returned by PROPFIND on a folder in a project were
   erroneous because ACL permissions were being ignored. This stems from a bug in
   grpcMDResponseToFileInfo, where the SysACL attribute of the FileInfo struct was not being
   populated.

   https://github.com/cs3org/reva/pull/4901
   see:

 * Bugfix #4907: Public links: return error when owner could not be resolved

   https://github.com/cs3org/reva/pull/4907

 * Bugfix #4591: Eos: fixed error reporting for too large recycle bin listing

   EOS returns E2BIG, which internally gets converted to PermissionDenied and has to be properly
   handled in this case.

   https://github.com/cs3org/reva/pull/4591

 * Bugfix #4896: Fix nilpointer error in RollbackToVersion

   https://github.com/cs3org/reva/pull/4896

 * Bugfix #4905: PurgeDate in ListDeletedEntries was ignored

   The date range that can be passed to ListDeletedEntries was not taken into account due to a bug in
   reva: the Purgedate argument was set, which only works for PURGE requests, and not for LIST
   requests. Instead, the Listflag argument must be used. Additionally, there was a bug in the
   loop that is used to iterate over all days in the date range.

   https://github.com/cs3org/reva/pull/4905

 * Bugfix #4939: Revert 'make home layout configurable'

   Partial revert of #4911, to be re-added after more testing and configuration validation. The
   eoshome vs eos storage drivers are to be adapted.

   https://github.com/cs3org/reva/pull/4939

 * Enhancement #5028: Handle empty EOS traces

   https://github.com/cs3org/reva/pull/5028

 * Enhancement #4911: Cephfs refactoring + make home layout configurable

   https://github.com/cs3org/reva/pull/4911

 * Enhancement #4937: @labkode steps down as project owner

   Hugo (@labkode) steps down as project owner of Reva.

   https://github.com/cs3org/reva/pull/4937

 * Enhancement #4579: Remove domain-specific code to other repos

   https://github.com/cs3org/reva/pull/4579

 * Enhancement #4824: Refactor Ceph code

   https://github.com/cs3org/reva/pull/4824

 * Enhancement #4797: Refactor CI jobs and bump to latest deps

   https://github.com/cs3org/reva/pull/4797

 * Enhancement #4934: Access to EOS via tokens over gRPC

   As a guest account, accessing a file shared with you relies on a token that is generated on behalf
   of the resource owner. This method, GenerateToken, has now been implemented in the EOS gRPC
   client. Additionally, the HTTP client now takes tokens into account.

   https://github.com/cs3org/reva/pull/4934

 * Enhancement #4870: Only load X509 on https

   Currently, the EOS HTTP Client always tries to read an X509 key pair from the file system (by
   default, from /etc/grid-security/host{key,cert}.pem). This makes it harder to write unit
   tests, as these fail when this key pair is not on the file system (which is the case for the test
   pipeline as well).

   This PR introduces a fix for this problem, by only loading the X509 key pair if the scheme of the
   EOS endpoint is https. Unit tests can then create a mock HTTP endpoint, which will not trigger
   the loading of the key pair.

   https://github.com/cs3org/reva/pull/4870

 * Enhancement #5014: Log app when creating EOS gRPC requests

   https://github.com/cs3org/reva/pull/5014

 * Enhancement #4892: Do not read eos user ACLs any longer

   This PR drops the compatibility code to read eos user ACLs in the eos binary client, and aligns it
   to the GRPC client.

   https://github.com/cs3org/reva/pull/4892

 * Enhancement #4720: Differentiate quota for user types in EOS

   We now assign a different initial quota to users depending on their type, whether PRIMARY or
   not.

   https://github.com/cs3org/reva/pull/4720

 * Enhancement #4863: Favourites for eos/grpc

   https://github.com/cs3org/reva/pull/4863

 * Enhancement #5013: Updated dependencies + moved to go 1.22

   https://github.com/cs3org/reva/pull/5013

 * Enhancement #4514: Pass lock holder metadata on uploads

   We now pass relevant metadata (lock id and lock holder) downstream on uploads, and handle the
   case of conflicts due to lock mismatch.

   https://github.com/cs3org/reva/pull/4514

 * Enhancement #4970: Improved logging on createHome

   https://github.com/cs3org/reva/pull/4970

 * Enhancement #4984: Drop shadow namespaces

   This comes as part of the effort to operate EOS without being root, see
   https://github.com/cs3org/reva/pull/4977

   In this PR the post-home creation hook (and corresponding flag) is replaced by a
   create_home_hook, and the following configuration parameters are suppressed:

   Shadow_namespace share_folder default_quota_bytes default_secondary_quota_bytes
   default_quota_files uploads_namespace (unused)

   https://github.com/cs3org/reva/pull/4984

 * Enhancement #4670: Ocm: support bearer token access

   This PR adds support for accessing remote OCM 1.1 shares via bearer token, as opposed to having
   the shared secret in the URL only. In addition, the OCM client package is now part of the OCMD
   server package, and the Discover methods have been all consolidated in one place.

   https://github.com/cs3org/reva/pull/4670

 * Enhancement #4977: Do not use root on EOS

   Currently, the EOS drivers use root authentication for many different operations. This has
   now been changed to use one of the following: * cbox, which is a sudo'er * daemon, for read-only
   operations * the user himselft

   Note that home creation is excluded here as this will be tackled in a different PR.

   https://github.com/cs3org/reva/pull/4977/


Changelog for reva 1.28.0 (2024-02-27)
=======================================

The following sections list the changes in reva 1.28.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4369: Carefully use root credentials to perform system level ops
 * Fix #4306: Correctly treat EOS urls containing # chars
 * Fix #4510: Propagates traceID to EOS
 * Fix #4321: Reworked List() to support version folder tricks
 * Fix #4400: Fix group-based capabilities
 * Fix #4319: Fixed registration of custom extensions in the mime registry
 * Fix #4287: Fixes registration and naming of services
 * Fix #4310: Restore changes to ceph driver
 * Fix #4294: Sciencemesh fixes
 * Fix #4307: Dynamic storage registry storage_id aliases
 * Fix #4497: Removed stat to all storage providers on Depth:0 PROPFIND to "/"
 * Enh #4280: Implementation of Locks for the CephFS driver
 * Enh #4282: Support multiple templates in config entries
 * Enh #4304: Disable open in app for given paths
 * Enh #4455: Limit max number of entries returned by ListRecycle in eos
 * Enh #4309: Get the logger in the grpcMDResponseToFileInfo func, log the stat
 * Enh #4311: Init time logger for eosgrpc storage driver
 * Enh #4301: Added listversions command
 * Enh #4493: Removed notification capability
 * Enh #4288: Print plugins' version
 * Enh #4508: Add pprof http service
 * Enh #4376: Removed cback from upstream codebase
 * Enh #4391: CERNBox setup for ScienceMesh tests
 * Enh #4246: Revamp ScienceMesh integration tests
 * Enh #4240: Reworked protocol with ScienceMesh NC/OC apps
 * Enh #4370: Storage registry: fail at init if config is missing any providers

Details
-------

 * Bugfix #4369: Carefully use root credentials to perform system level ops

   This PR ensures that system level ops like setlock, setattr, stat... work when invoked from a
   gateway This is relevant for eosgrpc, as eosbinary exploited the permissivity of the eos
   cmdline

   https://github.com/cs3org/reva/pull/4369

 * Bugfix #4306: Correctly treat EOS urls containing # chars

   https://github.com/cs3org/reva/pull/4306

 * Bugfix #4510: Propagates traceID to EOS

   This PR fixes the cases where the EOS trace ID was always a bunch of zeroes.

   https://github.com/cs3org/reva/pull/4510

 * Bugfix #4321: Reworked List() to support version folder tricks

   https://github.com/cs3org/reva/pull/4321

 * Bugfix #4400: Fix group-based capabilities

   The group-based capabilities require an authenticated endpoint, as we must query the
   logged-in user's groups to get those. This PR moves them to the `getSelf` endpoint in the user
   handler.

   https://github.com/cs3org/reva/pull/4400

 * Bugfix #4319: Fixed registration of custom extensions in the mime registry

   This PR ensures custom extensions/mime-types are registered by trimming any eventual
   leading '.' from the extension.

   https://github.com/cs3org/reva/pull/4319

 * Bugfix #4287: Fixes registration and naming of services

   https://github.com/cs3org/reva/pull/4287

 * Bugfix #4310: Restore changes to ceph driver

   PR [4166](https://github.com/cs3org/reva/pull/4166) accidentally reverted the ceph
   driver changes. This PR recovers them.

   https://github.com/cs3org/reva/pull/4310

 * Bugfix #4294: Sciencemesh fixes

   Fixes different issues introduced with the recent changes, in ocm/sciencemesh, in
   particular the `GetAccepetdUser` and `/sciencemesh/find-accepted-users` endpoints.

   https://github.com/cs3org/reva/pull/4294

 * Bugfix #4307: Dynamic storage registry storage_id aliases

   Fixes the bug where the dynamic storage registry would not be able to resolve storage ids like
   `eoshome-a`, as those are aliased and need to be resolved into the proper storage-id
   (`eoshome-i01`).

   https://github.com/cs3org/reva/pull/4307

 * Bugfix #4497: Removed stat to all storage providers on Depth:0 PROPFIND to "/"

   This PR removes an unnecessary and potentially problematic call, which would fail if any of the
   configured storage providers has an issue.

   https://github.com/cs3org/reva/pull/4497

 * Enhancement #4280: Implementation of Locks for the CephFS driver

   This PR brings CS3APIs Locks for CephFS

   https://github.com/cs3org/reva/pull/4280

 * Enhancement #4282: Support multiple templates in config entries

   This PR introduces support for config entries with multiple templates, such as `parameter =
   "{{ vars.v1 }} foo {{ vars.v2 }}"`. Previously, only one `{{ template }}` was allowed in a given
   configuration entry.

   https://github.com/cs3org/reva/pull/4282

 * Enhancement #4304: Disable open in app for given paths

   https://github.com/cs3org/reva/pull/4304

 * Enhancement #4455: Limit max number of entries returned by ListRecycle in eos

   The idea is to query first how many entries we'd have from eos recycle ls and bail out if "too
   many".

   https://github.com/cs3org/reva/pull/4455

 * Enhancement #4309: Get the logger in the grpcMDResponseToFileInfo func, log the stat

   https://github.com/cs3org/reva/pull/4309

 * Enhancement #4311: Init time logger for eosgrpc storage driver

   Before the `eosgrpc` driver was using a custom logger. Now that the reva logger is available at
   init time, the driver will use this.

   https://github.com/cs3org/reva/pull/4311

 * Enhancement #4301: Added listversions command

   https://github.com/cs3org/reva/pull/4301

 * Enhancement #4493: Removed notification capability

   This is not needed any longer, the code was simplified to enable notifications if they are
   configured

   https://github.com/cs3org/reva/pull/4493

 * Enhancement #4288: Print plugins' version

   https://github.com/cs3org/reva/pull/4288

 * Enhancement #4508: Add pprof http service

   This service is useful to trigger diagnostics on running processes

   https://github.com/cs3org/reva/pull/4508

 * Enhancement #4376: Removed cback from upstream codebase

   The code has been moved to as a CERNBox plugin.

   https://github.com/cs3org/reva/pull/4376

 * Enhancement #4391: CERNBox setup for ScienceMesh tests

   This PR includes a bundled CERNBox-like web UI and backend to test the ScienceMesh workflows
   with OC10 and NC

   https://github.com/cs3org/reva/pull/4391

 * Enhancement #4246: Revamp ScienceMesh integration tests

   This extends the ScienceMesh tests by running a wopiserver next to each EFSS/IOP, and by
   including a CERNBox-like minimal configuration. The latter is based on local storage and
   in-memory shares (no db dependency).

   https://github.com/cs3org/reva/pull/4246

 * Enhancement #4240: Reworked protocol with ScienceMesh NC/OC apps

   This ensures full OCM 1.1 coverage

   https://github.com/cs3org/reva/pull/4240

 * Enhancement #4370: Storage registry: fail at init if config is missing any providers

   This change makes the dynamic storage registry fail at startup if there are missing rules in the
   config file. That is, any `mount_id` in the routing table must have a corresponding
   `storage_id`/`address` pair in the config, otherwise the registry will fail to start.

   https://github.com/cs3org/reva/pull/4370


Changelog for reva 1.27.0 (2023-10-19)
=======================================

The following sections list the changes in reva 1.27.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4196: Access public links to projects as owner
 * Enh #4266: Improve authentication routing logic
 * Enh #4212: CERNBox cleanup
 * Enh #4199: Dynamic storage provider
 * Enh #4264: Implement eos-compliant app locks
 * Enh #4200: Multiple fixes for Ceph driver
 * Enh #4185: Refurbish the grpc and https plugins for eos
 * Enh #4166: Add better observability with metrics and traces
 * Enh #4195: Support incoming OCM 1.0 shares
 * Enh #4189: Support full URL endpoints in ocm-provider
 * Enh #4186: Fixes in the reference configuration for ScienceMesh
 * Enh #4191: Add metrics service to ScienceMesh example config

Details
-------

 * Bugfix #4196: Access public links to projects as owner

   https://github.com/cs3org/reva/pull/4196

 * Enhancement #4266: Improve authentication routing logic

   Provides a safer approach to route requests, both in HTTP and gRPC land when authentication is
   needed.

   https://github.com/cs3org/reva/pull/4266

 * Enhancement #4212: CERNBox cleanup

   Remove from the codebase all the cernbox specific code

   https://github.com/cs3org/reva/pull/4212

 * Enhancement #4199: Dynamic storage provider

   Add a new storage provider that can globally route to other providers. This provider uses a
   routing table in the database containing `path` - `mountid` pairs, and a mapping `mountid` -
   `address` in the config. It also support rewriting paths for resolution (to enable more
   complex cases).

   https://github.com/cs3org/reva/pull/4199

 * Enhancement #4264: Implement eos-compliant app locks

   The eosfs package now uses the app locks provided by eos

   https://github.com/cs3org/reva/pull/4264

 * Enhancement #4200: Multiple fixes for Ceph driver

   * Avoid usage/creation of user homes when they are disabled in the config * Simplify the regular
   uploads (not chunked) * Avoid creation of shadow folders at the root if they are already there *
   Clean up the chunked upload * Fix panic on shutdown

   https://github.com/cs3org/reva/pull/4200

 * Enhancement #4185: Refurbish the grpc and https plugins for eos

   This enhancement refurbishes the grpc and https plugins for eos

   https://github.com/cs3org/reva/pull/4185

 * Enhancement #4166: Add better observability with metrics and traces

   Adds prometheus collectors that can be registered dynamically and also refactors the http and
   grpc clients and servers to propage trace info.

   https://github.com/cs3org/reva/pull/4166

 * Enhancement #4195: Support incoming OCM 1.0 shares

   OCM 1.0 payloads are now supported as incoming shares, and converted to the OCM 1.1 format for
   persistency and further processing. Outgoing shares are still only OCM 1.1.

   https://github.com/cs3org/reva/pull/4195

 * Enhancement #4189: Support full URL endpoints in ocm-provider

   This patch enables a reva server to properly show any configured endpoint route in all relevant
   properties exposed by /ocm-provider. This allows reverse proxy configurations of the form
   https://server/route to be supported for the OCM discovery mechanism.

   https://github.com/cs3org/reva/pull/4189

 * Enhancement #4186: Fixes in the reference configuration for ScienceMesh

   Following the successful onboarding of CESNET, this PR brings some improvements and fixes to
   the reference configuration, as well as some adaptation to the itegration tests.

   https://github.com/cs3org/reva/pull/4186
   https://github.com/cs3org/reva/pull/4184
   https://github.com/cs3org/reva/pull/4183

 * Enhancement #4191: Add metrics service to ScienceMesh example config

   Adds the metrics http service configuration to the example config file of a ScienceMesh site.
   Having this service configured is a prerequisite for successfull Prometheus-based
   ScienceMesh sites metrics scraping.

   https://github.com/cs3org/reva/pull/4191


Changelog for reva 1.26.0 (2023-09-08)
=======================================

The following sections list the changes in reva 1.26.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4165: Use default user tmp folder in config tests
 * Fix #4113: Fix plugin's registration when reva is built with version 1.21
 * Fix #4171: Fix accessing an OCM-shared resource containing spaces
 * Fix #4172: Hardcode access methods for outgoing OCM shares from OC/NC
 * Fix #4125: Enable projects for lightweight accounts
 * Enh #4121: Expire cached users and groups entries
 * Enh #4162: Disable sharing on a storage provider
 * Enh #4163: Disable trashbin on a storage provider
 * Enh #4164: Disable versions on a storage provider
 * Enh #4084: Implementation of an app provider for Overleaf
 * Enh #4114: List all the registered plugins
 * Enh #4115: All required features and fixes for the OC/NC ScienceMesh apps

Details
-------

 * Bugfix #4165: Use default user tmp folder in config tests

   https://github.com/cs3org/reva/pull/4165

 * Bugfix #4113: Fix plugin's registration when reva is built with version 1.21

   With go 1.21 the logic for package initialization has changed, and the plugins were failing in
   the registration. Now the registration of the plugins is deferred in the main.

   https://github.com/cs3org/reva/pull/4113

 * Bugfix #4171: Fix accessing an OCM-shared resource containing spaces

   Fixes the access of a resource OCM-shared containing spaces, that previously was failing with
   a `NotFound` error.

   https://github.com/cs3org/reva/pull/4171

 * Bugfix #4172: Hardcode access methods for outgoing OCM shares from OC/NC

   This is a workaround until sciencemesh/nc-sciencemesh#45 is properly implemented

   https://github.com/cs3org/reva/pull/4172

 * Bugfix #4125: Enable projects for lightweight accounts

   Enable CERNBox projects to be listed by a lightweight account

   https://github.com/cs3org/reva/pull/4125

 * Enhancement #4121: Expire cached users and groups entries

   Entries in the rest user and group drivers do not expire. This means that old users/groups that
   have been deleted are still in cache. Now an expiration of `fetch interval + 1` hours has been
   set.

   https://github.com/cs3org/reva/pull/4121

 * Enhancement #4162: Disable sharing on a storage provider

   Added a GRPC interceptor that disable sharing permissions on a storage provider.

   https://github.com/cs3org/reva/pull/4162

 * Enhancement #4163: Disable trashbin on a storage provider

   Added a GRPC interceptor that disable the trashbin on a storage provider.

   https://github.com/cs3org/reva/pull/4163

 * Enhancement #4164: Disable versions on a storage provider

   Added a GRPC interceptor that disable the versions on a storage provider.

   https://github.com/cs3org/reva/pull/4164

 * Enhancement #4084: Implementation of an app provider for Overleaf

   This PR adds an app provider for Overleaf as a standalone http service.

   The app provider currently consists of support for the export to Overleaf feature, which when
   called returns a URL to Overleaf that prompts Overleaf to download the appropriate resource
   making use of the Archiver service, and upload the files to a user's Overleaf account.

   https://github.com/cs3org/reva/pull/4084

 * Enhancement #4114: List all the registered plugins

   https://github.com/cs3org/reva/pull/4114

 * Enhancement #4115: All required features and fixes for the OC/NC ScienceMesh apps

   This PR includes all necessary code in Reva to interface with the ScienceMesh apps in OC and NC

   https://github.com/cs3org/reva/pull/4115


Changelog for reva 1.25.0 (2023-08-14)
=======================================

The following sections list the changes in reva 1.25.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #4032: Temporarily exclude ceph-iscsi when building revad-ceph image
 * Fix #3883: Fix group request to Grappa
 * Fix #3946: Filter OCM shares by path
 * Fix #4016: Fix panic when closing notification service
 * Fix #4061: Fixes on notifications
 * Fix #3962: OCM-related compatibility fixes
 * Fix #3972: Fix for #3971
 * Fix #3882: Remove transfer on cancel should also remove transfer job
 * Chg #4041: Clean up notifications error checking code, fix sql creation script
 * Chg #3581: Remove meshdirectory http service
 * Enh #4044: Added an /app/notify endpoint for logging/tracking apps
 * Enh #3915: Storage drivers setup for datatx
 * Enh #3891: Provide data transfer size with datatx share
 * Enh #3905: Remove redundant config for invite_link_template
 * Enh #4031: Dump reva config on SIGUSR1
 * Enh #3954: Extend EOS metadata
 * Enh #3958: Make `/sciencemesh/find-accepted-users` response
 * Enh #3908: Removed support for forcelock
 * Enh #4011: Improve logging of HTTP requests
 * Enh #3407: Add init time logging to all services
 * Enh #4030: Support multiple token strategies in auth middleware
 * Enh #4015: New configuration
 * Enh #3825: Notifications framework
 * Enh #3969: Conditional notifications initialization
 * Enh #4077: Handle target in OpenInApp response
 * Enh #4073: Plugins
 * Enh #3937: Manage OCM shares
 * Enh #4035: Enforce/validate configuration of services

Details
-------

 * Bugfix #4032: Temporarily exclude ceph-iscsi when building revad-ceph image

   Due to `Package ceph-iscsi-3.6-1.el8.noarch.rpm is not signed` error when building the
   revad-ceph docker image, the package `ceph-iscsi` has been excluded from the dnf update. It
   will be included again once the pkg will be signed again.

   https://github.com/cs3org/reva/pull/4032

 * Bugfix #3883: Fix group request to Grappa

   The `url.JoinPath` call was returning an url-encoded string, turning `?` into `%3`. This
   caused the request to return 404.

   https://github.com/cs3org/reva/pull/3883

 * Bugfix #3946: Filter OCM shares by path

   Fixes the bug of duplicated OCM shares returned in the share with others response.

   https://github.com/cs3org/reva/pull/3946

 * Bugfix #4016: Fix panic when closing notification service

   If the connection to the nats server was not yet estabished, the service on close was panicking.
   This has been now fixed.

   https://github.com/cs3org/reva/pull/4016

 * Bugfix #4061: Fixes on notifications

   This is to align the code to the latest schema for notifications

   https://github.com/cs3org/reva/pull/4061

 * Bugfix #3962: OCM-related compatibility fixes

   Following analysis of OC and NC code to access a remote share, we must expose paths and not full
   URIs on the /ocm-provider endpoint. Also we fix a lookup issue with apps over OCM and update
   examples.

   https://github.com/cs3org/reva/pull/3962

 * Bugfix #3972: Fix for #3971

   Fixed panic described in #3971

   https://github.com/cs3org/reva/pull/3972

 * Bugfix #3882: Remove transfer on cancel should also remove transfer job

   https://github.com/cs3org/reva/issues/3881
   https://github.com/cs3org/reva/pull/3882

 * Change #4041: Clean up notifications error checking code, fix sql creation script

   https://github.com/cs3org/reva/pull/4041

 * Change #3581: Remove meshdirectory http service

   As of meshdirectory-web version 2.0.0, it is now implemented and deployed as a completely
   separate app, independent from Reva. We removed any deprecated meshdirectory-related code
   from Reva.

   https://github.com/cs3org/reva/pull/3581

 * Enhancement #4044: Added an /app/notify endpoint for logging/tracking apps

   The new endpoint serves to probe the health state of apps such as Microsoft Office Online, and it
   is expected to be called by the frontend upon successful loading of the document by the
   underlying app

   https://github.com/cs3org/reva/pull/4044

 * Enhancement #3915: Storage drivers setup for datatx

   https://github.com/cs3org/reva/issues/3914
   https://github.com/cs3org/reva/pull/3915

 * Enhancement #3891: Provide data transfer size with datatx share

   https://github.com/cs3org/reva/issues/2104
   https://github.com/cs3org/reva/pull/3891

 * Enhancement #3905: Remove redundant config for invite_link_template

   This is to drop invite_link_template from the OCM-related config. Now the provider_domain
   and mesh_directory_url config options are both mandatory in the sciencemesh http service,
   and the link is directly built out of the context.

   https://github.com/cs3org/reva/pull/3905

 * Enhancement #4031: Dump reva config on SIGUSR1

   Add an option to the runtime to dump the configuration on a file (default to
   `/tmp/reva-dump.toml` and configurable) when the process receives a SIGUSR1 signal.
   Eventual errors are logged in the log.

   https://github.com/cs3org/reva/pull/4031

 * Enhancement #3954: Extend EOS metadata

   This PR extend the EOS metadata with atime and ctime fields. This change is backwards
   compatible.

   https://github.com/cs3org/reva/pull/3954

 * Enhancement #3958: Make `/sciencemesh/find-accepted-users` response

   Consistent with delete user parameters

   https://github.com/cs3org/reva/pull/3958

 * Enhancement #3908: Removed support for forcelock

   This workaround is not needed any longer, see also the wopiserver.

   https://github.com/cs3org/reva/pull/3908

 * Enhancement #4011: Improve logging of HTTP requests

   Added request and response headers and removed redundant URL from the "http" messages

   https://github.com/cs3org/reva/pull/4011

 * Enhancement #3407: Add init time logging to all services

   https://github.com/cs3org/reva/pull/3407

 * Enhancement #4030: Support multiple token strategies in auth middleware

   Different HTTP services can in general support different token strategies for validating the
   reva token. In this context, without updating every single client a mono process deployment
   will never work. Now the HTTP auth middleware accepts in its configuration a token strategy
   chain, allowing to provide the reva token in multiple places (bearer auth, header).

   https://github.com/cs3org/reva/pull/4030

 * Enhancement #4015: New configuration

   Allow multiple driverts of the same service to be in the same toml config. Add a `vars` section to
   contain common parameters addressable using templates in the configuration of the different
   drivers. Support templating to reference values of other parameters in the configuration.
   Assign random ports to services where the address is not specified.

   https://github.com/cs3org/reva/pull/4015

 * Enhancement #3825: Notifications framework

   Adds a notifications framework to Reva.

   The new notifications service communicates with the rest of reva using NATS. It provides
   helper functions to register new notifications and to send them.

   Notification templates are provided in the configuration files for each service, and they are
   registered into the notifications service on initialization.

   https://github.com/cs3org/reva/pull/3825

 * Enhancement #3969: Conditional notifications initialization

   Notification helpers in services will not try to initalize if there is no specific
   configuration.

   https://github.com/cs3org/reva/pull/3969

 * Enhancement #4077: Handle target in OpenInApp response

   This PR adds the OpenInApp.target and AppProviderInfo.action properties to the respective
   responses (/app/open and /app/list), to support different app integrations. In addition,
   the archiver was extended to use the name of the file/folder as opposed to "download", and to
   include a query parameter to override the archive type, as it will be used in an upcoming app.

   https://github.com/cs3org/reva/pull/4077

 * Enhancement #4073: Plugins

   Adds a plugin system for allowing the creation of external plugins for different plugable
   components in reva, for example grpc drivers, http services and middlewares.

   https://github.com/cs3org/reva/pull/4073

 * Enhancement #3937: Manage OCM shares

   Implements the following item regarding OCM: - update of OCM shares in both grpc and ocs layer,
   allowing an user to update permissions and expiration of the share - deletion of OCM shares in
   both grpc and ocs layer - accept/reject of received OCM shares - remove accepted remote users

   https://github.com/cs3org/reva/pull/3937

 * Enhancement #4035: Enforce/validate configuration of services

   Every driver can now specify some validation rules on the configuration. If the validation
   rules are not respected, reva will bail out on startup with a clear error.

   https://github.com/cs3org/reva/pull/4035


Changelog for reva 1.24.0 (2023-05-11)
=======================================

The following sections list the changes in reva 1.24.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3805: Apps: fixed viewMode resolution
 * Fix #3771: Fix files sharing capabilities
 * Fix #3749: Fix persisting updates of received shares in json driver
 * Fix #3723: Fix revad docker images by enabling CGO
 * Fix #3765: Fix create version folder in EOS driver
 * Fix #3786: Fix listing directory for a read-only shares for EOS storage driver
 * Fix #3787: Fix application flag for EOS binary
 * Fix #3780: Fix Makefile error on Ubuntu
 * Fix #3873: Fix parsing of grappa response
 * Fix #3794: Fix unshare for EOS storage driver
 * Fix #3838: Fix upload in a single file share for lightweight accounts
 * Fix #3878: Fix creator/initiator in public and user shares
 * Fix #3813: Fix propfind URL for OCM shares
 * Fix #3770: Fixed default protocol on ocm-share-create
 * Fix #3852: Pass remote share id and shared secret in OCM call
 * Fix #3859: Fix inconsistency between data transfer protocol naming
 * Enh #3847: Update data transfers for current OCM shares implementation
 * Enh #3869: Datatx tutorial
 * Enh #3762: Denial and Resharing Default capabilities
 * Enh #3717: Disable sharing on low level paths
 * Enh #3766: Download file revisions
 * Enh #3733: Add support for static linking
 * Enh #3778: Add support to tag eos traffic
 * Enh #3868: Implement historical way of constructing OCM WebDAV URL
 * Enh #3719: Skip computing groups when fetching all groups from grappa
 * Enh #3783: Updated OCM tutorial
 * Enh #3750: New metadata flags
 * Enh #3839: Support multiple issuer in OIDC auth driver
 * Enh #3772: New OCM discovery endpoint
 * Enh #3619: Tests for invitation manager SQL driver
 * Enh #3757: Support OCM v1.0 schema
 * Enh #3695: Create OCM share from sciencemesh service
 * Enh #3722: List only valid OCM tokens
 * Enh #3821: Revamp user/group drivers and fix user type
 * Enh #3724: Send invitation link from mesh directory
 * Enh #3824: Serverless Services

Details
-------

 * Bugfix #3805: Apps: fixed viewMode resolution

   Currently, the viewMode passed on /app/open is taken without validating the actual user's
   permissions. This PR fixes this.

   https://github.com/cs3org/reva/pull/3805

 * Bugfix #3771: Fix files sharing capabilities

   A bug was preventing setting some capabilities (ResharingDefault and DenyAccess) for files
   sharing from the configuration file

   https://github.com/cs3org/reva/pull/3771

 * Bugfix #3749: Fix persisting updates of received shares in json driver

   https://github.com/cs3org/reva/pull/3749

 * Bugfix #3723: Fix revad docker images by enabling CGO

   https://github.com/cs3org/reva/pull/3723

 * Bugfix #3765: Fix create version folder in EOS driver

   In a read only share, a stat could fail, beacause the EOS storage driver was not able to create the
   version folder for a file in case this did not exist. This fixes this bug impersonating the owner
   of the file when creating the version folder.

   https://github.com/cs3org/reva/pull/3765

 * Bugfix #3786: Fix listing directory for a read-only shares for EOS storage driver

   In a read-only share, while listing a folder, for resources not having a version folder, the
   returned resource id was wrongly the one of the original file, instead of the version folder.
   This behavior has been fixed, where the version folder is always created on behalf of the
   resource owner.

   https://github.com/cs3org/reva/pull/3786

 * Bugfix #3787: Fix application flag for EOS binary

   https://github.com/cs3org/reva/pull/3787

 * Bugfix #3780: Fix Makefile error on Ubuntu

   I've fixed Makefile using sh which is defaulted to dash in ubuntu, dash doesn't support `[[ ...
   ]]` syntax and Makefile would throw `/bin/sh: 1: [[: not found` errors.

   https://github.com/cs3org/reva/issues/3773
   https://github.com/cs3org/reva/pull/3780

 * Bugfix #3873: Fix parsing of grappa response

   https://github.com/cs3org/reva/pull/3873

 * Bugfix #3794: Fix unshare for EOS storage driver

   In the EOS storage driver, the remove acl operation was a no-op. After removing a share, the
   recipient of the share was still able to operate on the shared resource. Now this has been fixed,
   removing correctly the ACL from the shared resource.

   https://github.com/cs3org/reva/pull/3794

 * Bugfix #3838: Fix upload in a single file share for lightweight accounts

   https://github.com/cs3org/reva/pull/3838

 * Bugfix #3878: Fix creator/initiator in public and user shares

   https://github.com/cs3org/reva/pull/3878

 * Bugfix #3813: Fix propfind URL for OCM shares

   https://github.com/cs3org/reva/issues/3810
   https://github.com/cs3org/reva/pull/3813

 * Bugfix #3770: Fixed default protocol on ocm-share-create

   https://github.com/cs3org/reva/pull/3770

 * Bugfix #3852: Pass remote share id and shared secret in OCM call

   https://github.com/cs3org/reva/pull/3852

 * Bugfix #3859: Fix inconsistency between data transfer protocol naming

   https://github.com/cs3org/reva/issues/3858
   https://github.com/cs3org/reva/pull/3859

 * Enhancement #3847: Update data transfers for current OCM shares implementation

   https://github.com/cs3org/reva/issues/3846
   https://github.com/cs3org/reva/pull/3847

 * Enhancement #3869: Datatx tutorial

   https://github.com/cs3org/reva/issues/3864
   https://github.com/cs3org/reva/pull/3869

 * Enhancement #3762: Denial and Resharing Default capabilities

   https://github.com/cs3org/reva/pull/3762

 * Enhancement #3717: Disable sharing on low level paths

   Sharing can be disable in the user share provider for some paths, but the storage provider was
   still sending the sharing permissions for those paths. This adds a config option in the storage
   provider, `minimum_allowed_path_level_for_share`, to disable sharing permissions for
   resources up to a defined path level.

   https://github.com/cs3org/reva/pull/3717

 * Enhancement #3766: Download file revisions

   Currently it is only possible to restore a file version, replacing the actual file with the
   selected version. This allows an user to download a version file, without touching/replacing
   the last version of the file

   https://github.com/cs3org/reva/pull/3766

 * Enhancement #3733: Add support for static linking

   We've added support for compiling reva with static linking enabled. It's possible to do so with
   the `STATIC` flag: `make revad STATIC=true`

   https://github.com/cs3org/reva/pull/3733

 * Enhancement #3778: Add support to tag eos traffic

   We've added support to tag eos traffic

   https://github.com/cs3org/reva/pull/3778

 * Enhancement #3868: Implement historical way of constructing OCM WebDAV URL

   Expose the expected WebDAV endpoint for OCM by OC10 and Nextcloud as described in
   https://github.com/cs3org/OCM-API/issues/70#issuecomment-1538551138 to allow reva
   providers to participate to mesh.

   https://github.com/cs3org/reva/issues/3855
   https://github.com/cs3org/reva/pull/3868

 * Enhancement #3719: Skip computing groups when fetching all groups from grappa

   https://github.com/cs3org/reva/pull/3719

 * Enhancement #3783: Updated OCM tutorial

   The OCM tutorial in the doc was missing the example on how to access the received resources. Now
   the tutorial contains all the steps to access a received resource using the WebDAV protocol.

   https://github.com/cs3org/reva/pull/3783

 * Enhancement #3750: New metadata flags

   Several new flags, like site infrastructure and service status, are now gathered and exposed
   by Mentix.

   https://github.com/cs3org/reva/pull/3750

 * Enhancement #3839: Support multiple issuer in OIDC auth driver

   The OIDC auth driver supports now multiple issuers. Users of external providers are then
   mapped to a local user by a mapping files. Only the main issuer (defined in the config with
   `issuer`) and the ones defined in the mapping are allowed for the verification of the OIDC
   token.

   https://github.com/cs3org/reva/pull/3839

 * Enhancement #3772: New OCM discovery endpoint

   This PR implements the new OCM v1.1 specifications for the /ocm-provider endpoint.

   https://github.com/cs3org/reva/pull/3772

 * Enhancement #3619: Tests for invitation manager SQL driver

   https://github.com/cs3org/reva/pull/3619

 * Enhancement #3757: Support OCM v1.0 schema

   Following cs3org/cs3apis#206, we add the fields to ensure backwards compatibility with OCM
   v1.0. However, if the `protocol.options` undocumented object is not empty, we bail out for
   now. Supporting interoperability with OCM v1.0 implementations (notably Nextcloud 25) may
   come in the future if the undocumented options are fully reverse engineered. This is reflected
   in the unit tests as well.

   Also, added viewMode to webapp protocol options (cs3org/cs3apis#207) and adapted all SQL
   code and unit tests.

   https://github.com/cs3org/reva/pull/3757

 * Enhancement #3695: Create OCM share from sciencemesh service

   https://github.com/pondersource/sciencemesh-php/issues/166
   https://github.com/cs3org/reva/pull/3695

 * Enhancement #3722: List only valid OCM tokens

   https://github.com/cs3org/reva/pull/3722

 * Enhancement #3821: Revamp user/group drivers and fix user type

   For lightweight accounts

   * Fix the user type for lightweight accounts, using the source field to differentiate between a
   primary and lw account * Remove all the code with manual parsing of the json returned by the CERN
   provider * Introduce pagination for `GetMembers` method in the group driver * Reduced network
   transfer size by requesting only needed fields for `GetMembers` method

   https://github.com/cs3org/reva/pull/3821

 * Enhancement #3724: Send invitation link from mesh directory

   When generating and listing OCM tokens

   To enhance user expirience, instead of only sending the token, we send directly the URL for
   accepting the invitation workflow.

   https://github.com/cs3org/reva/pull/3724

 * Enhancement #3824: Serverless Services

   New type of service (along with http and grpc) which does not have a listening server. Useful for
   the notifications service and others in the future.

   https://github.com/cs3org/reva/pull/3824


Changelog for reva 1.23.0 (2023-03-09)
=======================================

The following sections list the changes in reva 1.23.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3621: Use 2700 as permission when creating EOS home folder
 * Fix #3551: Fixes implementation omission of #3526
 * Fix #3706: Fix revad-eos docker image which was failing to build
 * Fix #3626: Fix open in app for lightweight accounts
 * Fix #3613: Use subject from oidc userinfo when quering the user provider
 * Fix #3633: Fix litmus and acceptance tests in GitHub Actions
 * Fix #3694: Updated public links URLs and users' display names in WOPI apps
 * Chg #3553: Rename PullTransfer to CreateTransfer
 * Enh #3584: Bump the Copyright date to 2023
 * Enh #3640: Migrate acceptance tests from Drone to GitHub Actions
 * Enh #3629: Use cs3org/behat:latest docker image for tests
 * Enh #3608: Add Golang test coverage report for Codacy
 * Enh #3599: Add latest tag to revad Docker image with GitHub Actions
 * Enh #3713: Streamline EOS SSS and UNIX modes
 * Enh #3566: Migrate the litmusOcisSpacesDav test from Drone to GitHub Actions
 * Enh #3712: Improve Docker build speed and Docker Compose test speed
 * Enh #3630: Migrate the virtualViews test from Drone to GitHub Actions
 * Enh #3675: Cleanup unused configs in OCM HTTP service
 * Enh #3692: Create and list OCM shares in OCS layer
 * Enh #3666: Search OCM accepted users
 * Enh #3665: List valid OCM invite tokens
 * Enh #3617: SQL driver for OCM invitation manager
 * Enh #3667: List OCM providers
 * Enh #3668: Expose OCM received shares as a local mount
 * Enh #3683: Remote open in app in OCM
 * Enh #3654: SQL driver for OCM shares
 * Enh #3646: Update OCM shares to last version of CS3APIs
 * Enh #3687: Specify recipient as a query param when sending OCM token by email
 * Enh #3691: Add OCM scope and webdav endpoint
 * Enh #3611: Revamp OCM invitation workflow
 * Enh #3703: Bump reva(d) base image to alpine 3.17

Details
-------

 * Bugfix #3621: Use 2700 as permission when creating EOS home folder

   https://github.com/cs3org/reva/pull/3621

 * Bugfix #3551: Fixes implementation omission of #3526

   In #3526 a new value format of the owner parameter of the ocm share request was introduced. This
   change was not implemented in the json driver. This change fixes that.

   https://github.com/cs3org/reva/pull/3551

 * Bugfix #3706: Fix revad-eos docker image which was failing to build

   https://github.com/cs3org/reva/pull/3706

 * Bugfix #3626: Fix open in app for lightweight accounts

   https://github.com/cs3org/reva/pull/3626

 * Bugfix #3613: Use subject from oidc userinfo when quering the user provider

   https://github.com/cs3org/reva/pull/3613

 * Bugfix #3633: Fix litmus and acceptance tests in GitHub Actions

   https://github.com/cs3org/reva/pull/3633

 * Bugfix #3694: Updated public links URLs and users' display names in WOPI apps

   Public links have changed in the frontend and are reflected in folderurl query parameter.
   Additionally, OCM shares are supported for the folderurl and OCM users are decorated with
   their ID provider.

   https://github.com/cs3org/reva/pull/3694

 * Change #3553: Rename PullTransfer to CreateTransfer

   This change implements a CS3APIs name change in the datatx module (PullTransfer to
   CreateTransfer)

   https://github.com/cs3org/reva/pull/3553

 * Enhancement #3584: Bump the Copyright date to 2023

   https://github.com/cs3org/reva/pull/3584

 * Enhancement #3640: Migrate acceptance tests from Drone to GitHub Actions

   Migrate ocisIntegrationTests and s3ngIntegrationTests to GitHub Actions

   https://github.com/cs3org/reva/pull/3640

 * Enhancement #3629: Use cs3org/behat:latest docker image for tests

   https://github.com/cs3org/reva/pull/3629

 * Enhancement #3608: Add Golang test coverage report for Codacy

   https://github.com/cs3org/reva/pull/3608

 * Enhancement #3599: Add latest tag to revad Docker image with GitHub Actions

   https://github.com/cs3org/reva/pull/3599

 * Enhancement #3713: Streamline EOS SSS and UNIX modes

   https://github.com/cs3org/reva/pull/3713

 * Enhancement #3566: Migrate the litmusOcisSpacesDav test from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3566

 * Enhancement #3712: Improve Docker build speed and Docker Compose test speed

   https://github.com/cs3org/reva/pull/3712

 * Enhancement #3630: Migrate the virtualViews test from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3630

 * Enhancement #3675: Cleanup unused configs in OCM HTTP service

   https://github.com/cs3org/reva/pull/3675

 * Enhancement #3692: Create and list OCM shares in OCS layer

   https://github.com/cs3org/reva/pull/3692

 * Enhancement #3666: Search OCM accepted users

   Adds the prefix `sm:` to the FindUser endpoint, to filter only the OCM accepted users.

   https://github.com/cs3org/reva/pull/3666

 * Enhancement #3665: List valid OCM invite tokens

   Adds the endpoint `/list-invite` in the sciencemesh service, to get the list of valid OCM
   invite tokens.

   https://github.com/cs3org/reva/pull/3665
   https://github.com/cs3org/cs3apis/pull/201

 * Enhancement #3617: SQL driver for OCM invitation manager

   https://github.com/cs3org/reva/pull/3617

 * Enhancement #3667: List OCM providers

   Adds the endpoint `/list-providers` in the sciencemesh service, to get a filtered list of the
   OCM providers. The filter can be specified with the `search` query parameters, and filters by
   domain and full name of the provider.

   https://github.com/cs3org/reva/pull/3667

 * Enhancement #3668: Expose OCM received shares as a local mount

   https://github.com/cs3org/reva/pull/3668

 * Enhancement #3683: Remote open in app in OCM

   https://github.com/cs3org/reva/pull/3683

 * Enhancement #3654: SQL driver for OCM shares

   https://github.com/cs3org/reva/pull/3654

 * Enhancement #3646: Update OCM shares to last version of CS3APIs

   https://github.com/cs3org/reva/pull/3646
   https://github.com/cs3org/cs3apis/pull/199

 * Enhancement #3687: Specify recipient as a query param when sending OCM token by email

   Before the email recipient when sending the OCM token was specified as a form parameter. Now as a
   query parameter, as some clients does not allow in a GET request to set form values. It also add
   the possibility to specify a template for the subject and the body for the token email.

   https://github.com/cs3org/reva/pull/3687

 * Enhancement #3691: Add OCM scope and webdav endpoint

   Adds the OCM scope and the ocmshares authentication, to authenticate the federated user to use
   the OCM shared resources. It also adds the (unprotected) webdav endpoint used to interact with
   the shared resources.

   https://github.com/cs3org/reva/issues/2739
   https://github.com/cs3org/reva/pull/3691

 * Enhancement #3611: Revamp OCM invitation workflow

   https://github.com/cs3org/reva/issues/3540
   https://github.com/cs3org/reva/pull/3611

 * Enhancement #3703: Bump reva(d) base image to alpine 3.17

   Prevents several vulnerabilities from the base image itself:
   https://artifacthub.io/packages/helm/cs3org/revad?modal=security-report

   https://github.com/cs3org/reva/pull/3703


Changelog for reva 1.22.0 (2022-12-31)
=======================================

The following sections list the changes in reva 1.22.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3528: Fix expired authenticated public link error code
 * Fix #3121: Add missing domain normalization to mentix provider authorizer
 * Enh #3565: Migrate the litmus tests from Drone to GitHub Actions

Details
-------

 * Bugfix #3528: Fix expired authenticated public link error code

   On an expired authenticated public link, the error returned was 401 unauthorized, behaving
   differently from a not-authenticated one, that returns 404 not found. This has been fixed,
   returning 404 not found.

   https://github.com/cs3org/reva/pull/3528

 * Bugfix #3121: Add missing domain normalization to mentix provider authorizer

   The Mentix OCM Provider authorizer lacked provider domain normalization. This led to
   incorrect provider domain matching when authorizing OCM providers.

   https://github.com/cs3org/reva/pull/3121

 * Enhancement #3565: Migrate the litmus tests from Drone to GitHub Actions

   We've migrated the litmusOcisOldWebdav and the litmusOcisNewWebdav tests from Drone to
   GitHub Actions.

   https://github.com/cs3org/reva/pull/3565


Changelog for reva 1.21.0 (2022-12-07)
=======================================

The following sections list the changes in reva 1.21.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3492: Fixes the DefaultQuotaBytes in EOS
 * Fix #3420: EOS grpc fixes
 * Fix #3501: Fix errors of public share provider according to cs3apis
 * Fix #3504: Fix RefreshLock method for cephfs storage driver
 * Enh #3502: Appproviders: pass other query parameters as Opaque
 * Enh #3028: Access directly auth registry rules map when getting provider
 * Enh #3197: Bring back multi-stage build to save on image size
 * Enh #3506: Migrate the buildAndPublishDocker job from Drone to GitHub Actions
 * Enh #3500: Migrate the BuildOnly job from Drone to GitHub Actions
 * Enh #3513: Migrate the testIntegration job from Drone to GitHub Actions
 * Enh #3494: Implemented folderurl for WOPI apps
 * Enh #3507: Get user preferred language
 * Enh #3530: Improve error logging in ocmd flow
 * Enh #3491: Implement rclone third-party copy push option
 * Enh #3508: Allow an user to set a preferred language

Details
-------

 * Bugfix #3492: Fixes the DefaultQuotaBytes in EOS

   We were setting the default logical quota to 1T, resulting on only 500GB available to the user.

   https://github.com/cs3org/reva/pull/3492

 * Bugfix #3420: EOS grpc fixes

   The shares and the applications were not working with the EOS grpc storage driver. This fixes
   both.

   https://github.com/cs3org/reva/pull/3420

 * Bugfix #3501: Fix errors of public share provider according to cs3apis

   All the errors returned by the public share provider where internal errors. Now this has been
   fixed and the returned errors are the one defined in the cs3apis.

   https://github.com/cs3org/reva/pull/3501

 * Bugfix #3504: Fix RefreshLock method for cephfs storage driver

   https://github.com/cs3org/reva/pull/3504

 * Enhancement #3502: Appproviders: pass other query parameters as Opaque

   This allows to send any other HTTP query parameter passed to /app/open to the underlying
   appprovider drivers via GRPC

   https://github.com/cs3org/reva/pull/3502

 * Enhancement #3028: Access directly auth registry rules map when getting provider

   https://github.com/cs3org/reva/pull/3028

 * Enhancement #3197: Bring back multi-stage build to save on image size

   - Use EOS 4.8.91 as base image - Bring back multi-stage build - Build revad on the eos 4.8.91 image
   due to missing dependency (`ld-musl-x86_64.so.1`, typical of alpine) - Copy the resulting
   revad from the builder container

   Resulting image size (unpacked on disk) is 2.59GB - eos-all:4.8.91 is 2.47GB - existing
   revad:latest-eos is 6.18GB

   https://github.com/cs3org/reva/pull/3197

 * Enhancement #3506: Migrate the buildAndPublishDocker job from Drone to GitHub Actions

   We've migrated the buildAndPublishDocker job from Drone to GitHub Actions workflow. We've
   updated the Golang version used to build the Docker images to go1.19. We've fixed the Cephfs
   storage module. We've improved the Makefile. We've refactored the build-docker workflow.

   https://github.com/cs3org/reva/pull/3506

 * Enhancement #3500: Migrate the BuildOnly job from Drone to GitHub Actions

   We've migrated the BuildOnly job from Drone to GitHub Actions workflow. The Workflow builds
   and Tests Reva, builds a Revad Docker Image and checks the license headers. The license header
   tools was removed since the goheader linter provides the same functionality.

   https://github.com/cs3org/reva/pull/3500

 * Enhancement #3513: Migrate the testIntegration job from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3513

 * Enhancement #3494: Implemented folderurl for WOPI apps

   The folderurl is now populated for WOPI apps, such that for owners and named shares it points to
   the containing folder, and for public links it points to the appropriate public link URL.

   On the way, functions to manipulate the user's scope and extract the eventual public link
   token(s) have been added, coauthored with @gmgigi96.

   https://github.com/cs3org/reva/pull/3494

 * Enhancement #3507: Get user preferred language

   The only way for an OCIS web user to change language was to set it into the browser settings. In the
   ocs user info response, a field `language` is added, to change their language in the UI,
   regardless of the browser settings.

   https://github.com/cs3org/reva/pull/3507

 * Enhancement #3530: Improve error logging in ocmd flow

   https://github.com/cs3org/reva/issues/3365
   https://github.com/cs3org/reva/pull/3530
   https://github.com/cs3org/reva/pull/3526
   https://github.com/cs3org/reva/pull/3419
   https://github.com/cs3org/reva/pull/3369

 * Enhancement #3491: Implement rclone third-party copy push option

   This enhancement gives the option to use third-party copy push with rclone between two
   different user accounts.

   https://github.com/cs3org/reva/pull/3491

 * Enhancement #3508: Allow an user to set a preferred language

   https://github.com/cs3org/reva/pull/3508


Changelog for reva 1.20.0 (2022-11-24)
=======================================

The following sections list the changes in reva 1.20.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Sec #3316: Mitigate XSS
 * Fix #3455: Fixes panic in case of empty configuration
 * Fix #3311: Remove FIXME
 * Fix #3396: Fix the Ceph Docker image repository URL
 * Fix #3055: Fix quota for LW accounts
 * Fix #3361: Use custom reva logger in ocs
 * Fix #3344: Fix quota percentage
 * Fix #2979: Removed unused datatx code
 * Fix #2973: Fix datatxtarget uri when prefix is used
 * Fix #3319: Fix oidc provider crash when custom claims are provided
 * Fix #3481: OIDC: resolve users with no uid/gid by username
 * Fix #3055: Get user from user provider in oidc driver
 * Fix #3053: Temporary read user acl instead of sys acl
 * Enh #3401: Make WOPI bridged apps (CodiMD) configuration non hard-coded
 * Enh #3402: Block users
 * Enh #3098: App provider http endpoint uses Form instead of Query
 * Enh #3116: Implementation of cback storage driver for REVA
 * Enh #3422: Migrate Codacy from Drone to Codacy/GitHub integration
 * Enh #3412: Migrate Fossa from Drone to Github Integration
 * Enh #3367: Update go version
 * Enh #3467: Enable gocritic linter in golangci-lint and solve issues
 * Enh #3463: Enable gofmt linter in golangci-lint and apply gofmt
 * Enh #3471: Enable goimports and usestdlibvars in golangci-lint
 * Enh #3466: Migrate golangci-lint from Drone to GitHub Actions
 * Enh #3465: Enable revive linter in golangci-lint and solve issues
 * Enh #3487: Enable staticcheck linter in golangci-lint and solve issues
 * Enh #3475: Enable the style linters
 * Enh #3070: Allow http service to expose prefixes containing /
 * Enh #2986: Better display name in apps for all user types
 * Enh #3303: Added support for configuring language locales in apps
 * Enh #3348: Revamp lightweigth accounts
 * Enh #3304: Add http service to send email for shares
 * Enh #3072: Mesh meta data operators
 * Enh #3313: Fix content-type for OCM sharing
 * Enh #3234: Add post create home hook for eos storage driver
 * Enh #3347: Implemented PROPFIND with 0 depth
 * Enh #3056: Add public share auth provider
 * Enh #3305: Add description to public link
 * Enh #3163: Add support for quicklinks for public shares
 * Enh #3289: Make Refresh Lock operation WOPI compliant
 * Enh #3315: Accept reva token as a bearer authentication
 * Enh #3438: Sanitize non-utf8 characters in xattr values in EOS
 * Enh #3221: Site Accounts improvements
 * Enh #3404: Site accounts & Mentix updates
 * Enh #3424: Expire tokens on sunday
 * Enh #2986: Use email as display name for external users opening WOPI apps

Details
-------

 * Security #3316: Mitigate XSS

   We've mitigated an XSS vulnerability resulting from unescaped HTTP responses containing
   user-provided values in pkg/siteacc/siteacc.go and
   internal/http/services/ocmd/invites.go. This patch uses html.EscapeString to escape the
   user-provided values in the HTTP responses of pkg/siteacc/siteacc.go and
   internal/http/services/ocmd/invites.go.

   https://github.com/cs3org/reva/pull/3316

 * Bugfix #3455: Fixes panic in case of empty configuration

   Makes sure the config map is allocated prior to setting it

   https://github.com/cs3org/reva/pull/3455

 * Bugfix #3311: Remove FIXME

   Issue https://github.com/cs3org/reva/issues/2402 is closed.

   https://github.com/cs3org/reva/pull/3311

 * Bugfix #3396: Fix the Ceph Docker image repository URL

   https://github.com/cs3org/reva/pull/3396

 * Bugfix #3055: Fix quota for LW accounts

   LW accounts do not have quota assigned.

   https://github.com/cs3org/reva/pull/3055

 * Bugfix #3361: Use custom reva logger in ocs

   https://github.com/cs3org/reva/pull/3361

 * Bugfix #3344: Fix quota percentage

   https://github.com/cs3org/reva/pull/3344

 * Bugfix #2979: Removed unused datatx code

   An OCM reference is not created for a data transfer type share.

   https://github.com/cs3org/reva/pull/2979

 * Bugfix #2973: Fix datatxtarget uri when prefix is used

   When a webdav prefix is used it appears in both host and name parameter of the target uri for data
   transfer. This PR fixes that.

   https://github.com/cs3org/reva/pull/2973

 * Bugfix #3319: Fix oidc provider crash when custom claims are provided

   https://github.com/cs3org/reva/pull/3319

 * Bugfix #3481: OIDC: resolve users with no uid/gid by username

   Previously we resolved such users (so called "lightweight" or "external" accounts in the CERN
   realm) by email, but it turns out that the same email may have multiple accounts associated to
   it.

   Therefore we now resolve them by username, that is the upn, which is unique.

   https://github.com/cs3org/reva/pull/3481

 * Bugfix #3055: Get user from user provider in oidc driver

   For oidc providers that only respond with standard claims, use the user provider to get the
   user.

   https://github.com/cs3org/reva/pull/3055

 * Bugfix #3053: Temporary read user acl instead of sys acl

   We read the user acl in EOS until the migration of all user acls to sys acls are done

   https://github.com/cs3org/reva/pull/3053

 * Enhancement #3401: Make WOPI bridged apps (CodiMD) configuration non hard-coded

   The configuration of the custom mimetypes has been moved to the AppProvider, and the given
   mimetypes are used to configure bridged apps by sharing the corresponding config item to the
   drivers.

   https://github.com/cs3org/reva/pull/3401

 * Enhancement #3402: Block users

   Allows an operator to set a list of users that are banned for every operation in reva.

   https://github.com/cs3org/reva/pull/3402

 * Enhancement #3098: App provider http endpoint uses Form instead of Query

   We've improved the http endpoint now uses the Form instead of Query to also support
   `application/x-www-form-urlencoded` parameters on the app provider http endpoint.

   https://github.com/cs3org/reva/pull/3098
   https://github.com/cs3org/reva/pull/3101

 * Enhancement #3116: Implementation of cback storage driver for REVA

   This is a read only fs interface.

   https://github.com/cs3org/reva/pull/3116

 * Enhancement #3422: Migrate Codacy from Drone to Codacy/GitHub integration

   https://github.com/cs3org/reva/pull/3422

 * Enhancement #3412: Migrate Fossa from Drone to Github Integration

   https://github.com/cs3org/reva/pull/3412

 * Enhancement #3367: Update go version

   Update go version to 1.19 in go.mod

   https://github.com/cs3org/reva/pull/3367

 * Enhancement #3467: Enable gocritic linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3467

 * Enhancement #3463: Enable gofmt linter in golangci-lint and apply gofmt

   https://github.com/cs3org/reva/pull/3463

 * Enhancement #3471: Enable goimports and usestdlibvars in golangci-lint

   We've enabled the goimports and usestdlibvars linters in golangci-lint and solved the
   related issues.

   https://github.com/cs3org/reva/pull/3471

 * Enhancement #3466: Migrate golangci-lint from Drone to GitHub Actions

   https://github.com/cs3org/reva/pull/3466

 * Enhancement #3465: Enable revive linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3465

 * Enhancement #3487: Enable staticcheck linter in golangci-lint and solve issues

   https://github.com/cs3org/reva/pull/3487

 * Enhancement #3475: Enable the style linters

   We've enabled the stylecheck, whitespace, dupword, godot and dogsled linters in
   golangci-lint and solved the related issues.

   https://github.com/cs3org/reva/pull/3475

 * Enhancement #3070: Allow http service to expose prefixes containing /

   https://github.com/cs3org/reva/pull/3070

 * Enhancement #2986: Better display name in apps for all user types

   This includes a `FirstName FamilyName (domain)` format for non-primary accounts, and a
   sanitization of the email address claim for such non-primary accounts.

   https://github.com/cs3org/reva/pull/2986
   https://github.com/cs3org/reva/pull/3280

 * Enhancement #3303: Added support for configuring language locales in apps

   This is a partial backport from edge: we introduce a language option in the appprovider, which
   if set is passed as appropriate parameter to the external apps in order to force a given
   localization. In particular, for Microsoft Office 365 the DC_LLCC option is set as well. The
   default behavior is unset, where apps try and resolve the localization from the browser
   headers.

   https://github.com/cs3org/reva/pull/3303

 * Enhancement #3348: Revamp lightweigth accounts

   Re-implements the lighweight account scope check, making it more efficient. Also, the ACLs
   for the EOS storage driver for the lw accounts are set atomically.

   https://github.com/cs3org/reva/pull/3348

 * Enhancement #3304: Add http service to send email for shares

   https://github.com/cs3org/reva/pull/3304

 * Enhancement #3072: Mesh meta data operators

   To better support sites that run multiple instances, the meta data have been extended to
   include a new hierarchy layer called 'operators'. This PR brings all necessary changes in the
   Mentix and site accounts services.

   https://github.com/cs3org/reva/pull/3072

 * Enhancement #3313: Fix content-type for OCM sharing

   This fix change the content type to just "application/json"

   https://github.com/cs3org/reva/pull/3313

 * Enhancement #3234: Add post create home hook for eos storage driver

   https://github.com/cs3org/reva/pull/3234

 * Enhancement #3347: Implemented PROPFIND with 0 depth

   https://github.com/cs3org/reva/pull/3347

 * Enhancement #3056: Add public share auth provider

   Add a public share auth middleware

   https://github.com/cs3org/reva/pull/3056

 * Enhancement #3305: Add description to public link

   https://github.com/cs3org/reva/pull/3305

 * Enhancement #3163: Add support for quicklinks for public shares

   https://github.com/cs3org/reva/pull/3163
   https://github.com/cs3org/reva/pull/2715

 * Enhancement #3289: Make Refresh Lock operation WOPI compliant

   We now support the WOPI compliant `UnlockAndRelock` operation. This has been implemented in
   the Eos FS. To make use of it, we need a compatible WOPI server.

   https://github.com/cs3org/reva/pull/3289
   https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/rest/files/unlockandrelock

 * Enhancement #3315: Accept reva token as a bearer authentication

   https://github.com/cs3org/reva/pull/3315

 * Enhancement #3438: Sanitize non-utf8 characters in xattr values in EOS

   https://github.com/cs3org/reva/pull/3438

 * Enhancement #3221: Site Accounts improvements

   The site accounts admin panel has been reworked and now also shows which sites aren't
   configured properly yet. Furthermore, a bug that prevented users from changing site
   configurations has been fixed.

   https://github.com/cs3org/reva/pull/3221

 * Enhancement #3404: Site accounts & Mentix updates

   Some small improvements to the Site Accounts and Mentix services, including normalization of
   data exposed at the `/cs3` endpoint of Mentix.

   https://github.com/cs3org/reva/pull/3404

 * Enhancement #3424: Expire tokens on sunday

   https://github.com/cs3org/reva/pull/3424

 * Enhancement #2986: Use email as display name for external users opening WOPI apps

   We use now the email claim for external/federated accounts as the `username` that is then
   passed to the wopiserver and used as `displayName` in the WOPI context.

   https://github.com/cs3org/reva/pull/2986


Changelog for reva 1.19.0 (2022-06-16)
=======================================

The following sections list the changes in reva 1.19.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2693: Support editnew actions from MS Office
 * Fix #2588: Dockerfile.revad-ceph to use the right base image
 * Fix #2216: Make hardcoded HTTP "insecure" options configurable
 * Fix #2860: Use `eos-all` parent image
 * Fix #2499: Removed check DenyGrant in resource permission
 * Fix #2712: Update Dockerfile.revad.eos to not break the image
 * Fix #2789: Minor fixes in cephfs and eosfs
 * Fix #2285: Accept new userid idp format
 * Fix #2608: Respect the tracing_service_name config variable
 * Fix #2841: Refactors logger to have ctx
 * Fix #2759: Made uid, gid claims parsing more robust in OIDC auth provider
 * Fix #2842: Fix download action in SDK
 * Fix #2555: Fix site accounts endpoints
 * Fix #2675: Updates Makefile according to latest go standards
 * Fix #2572: Wait for nats server on middleware start
 * Chg #2596: Remove hash from public link urls
 * Chg #2559: Do not encode webDAV ids to base64
 * Chg #2561: Merge oidcmapping auth manager into oidc
 * Enh #2698: Make capabilities endpoint public, authenticate users is present
 * Enh #2813: Support custom mimetypes in the WOPI appprovider driver
 * Enh #2515: Enabling tracing by default if not explicitly disabled
 * Enh #160: Implement the CS3 Lock API in the EOS storage driver
 * Enh #2686: Features for favorites xattrs in EOS, cache for scope expansion
 * Enh #2494: Use sys ACLs for file permissions
 * Enh #2522: Introduce events
 * Enh #2685: Enable federated account access
 * Enh #2801: Use functional options for client gRPC connections
 * Enh #2921: Use standard header for checksums
 * Enh #2480: Group based capabilities
 * Enh #1787: Add support for HTTP TPC
 * Enh #2560: Mentix PromSD extensions
 * Enh #2613: Externalize custom mime types configuration for storage providers
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2696: Preferences driver refactor and cbox sql implementation
 * Enh #2052: New CS3API datatx methods
 * Enh #2738: Site accounts site-global settings
 * Enh #2672: Further Site Accounts improvements
 * Enh #2549: Site accounts improvements
 * Enh #2488: Cephfs support keyrings with IDs
 * Enh #2514: Reuse ocs role objects in other drivers
 * Enh #2752: Refactor the rest user and group provider drivers
 * Enh #2946: Make user share indicators read from the share provider service

Details
-------

 * Bugfix #2693: Support editnew actions from MS Office

   This fixes the incorrect behavior when creating new xlsx and pptx files, as MS Office supports
   the editnew action and it must be used for newly created files instead of the normal edit action.

   https://github.com/cs3org/reva/pull/2693

 * Bugfix #2588: Dockerfile.revad-ceph to use the right base image

   In Aug2021 https://hub.docker.com/r/ceph/daemon-base was moved to quay.ceph.io and the
   builds for this image were failing for some weeks after January.

   https://github.com/cs3org/reva/pull/2588

 * Bugfix #2216: Make hardcoded HTTP "insecure" options configurable

   HTTP "insecure" options must be configurable and default to false.

   https://github.com/cs3org/reva/issues/2216

 * Bugfix #2860: Use `eos-all` parent image

   https://github.com/cs3org/reva/pull/2860

 * Bugfix #2499: Removed check DenyGrant in resource permission

   When adding a denial permission

   https://github.com/cs3org/reva/pull/2499

 * Bugfix #2712: Update Dockerfile.revad.eos to not break the image

   https://github.com/cs3org/reva/pull/2712

 * Bugfix #2789: Minor fixes in cephfs and eosfs

   https://github.com/cs3org/reva/pull/2789

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

 * Bugfix #2608: Respect the tracing_service_name config variable

   https://github.com/cs3org/reva/pull/2608

 * Bugfix #2841: Refactors logger to have ctx

   This fixes the native library loggers which are not associated with the context and thus are not
   handled properly in the reva runtime.

   https://github.com/cs3org/reva/pull/2841

 * Bugfix #2759: Made uid, gid claims parsing more robust in OIDC auth provider

   This fix makes sure the uid and gid claims are defined at init time, and that the necessary
   typecasts are performed correctly when authenticating users. A comment was added that in case
   the uid/gid claims are missing AND that no mapping takes place, a user entity is returned with
   uid = gid = 0.

   https://github.com/cs3org/reva/pull/2759

 * Bugfix #2842: Fix download action in SDK

   The download action was no longer working in the SDK (used by our testing probes); this PR fixes
   the underlying issue.

   https://github.com/cs3org/reva/pull/2842

 * Bugfix #2555: Fix site accounts endpoints

   This PR fixes small bugs in the site accounts endpoints.

   https://github.com/cs3org/reva/pull/2555

 * Bugfix #2675: Updates Makefile according to latest go standards

   Earlier, we were using go get to install packages. Now, we are using go install to install
   packages

   https://github.com/cs3org/reva/issues/2675
   https://github.com/cs3org/reva/pull/2747

 * Bugfix #2572: Wait for nats server on middleware start

   Use a retry mechanism to connect to the nats server when it is not ready yet

   https://github.com/cs3org/reva/pull/2572

 * Change #2596: Remove hash from public link urls

   Public link urls do not contain the hash anymore, this is needed to support the ocis and web
   history mode.

   https://github.com/cs3org/reva/pull/2596
   https://github.com/owncloud/ocis/pull/3109
   https://github.com/owncloud/web/pull/6363

 * Change #2559: Do not encode webDAV ids to base64

   We removed the base64 encoding of the IDs and use the format <storageID>!<opaqueID> with a `!`
   delimiter. As a reserved delimiter it is URL safe. The IDs will be XML and JSON encoded as
   necessary.

   https://github.com/cs3org/reva/pull/2559

 * Change #2561: Merge oidcmapping auth manager into oidc

   The oidcmapping auth manager was created as a separate package to ease testing. As it has now
   been tested also as a pure OIDC auth provider without mapping, and as the code is largely
   refactored, it makes sense to merge it back so to maintain a single OIDC manager.

   https://github.com/cs3org/reva/pull/2561

 * Enhancement #2698: Make capabilities endpoint public, authenticate users is present

   https://github.com/cs3org/reva/pull/2698

 * Enhancement #2813: Support custom mimetypes in the WOPI appprovider driver

   Similarly to the storage provider, also the WOPI appprovider driver now supports custom mime
   types. Also fixed a small typo.

   https://github.com/cs3org/reva/pull/2813

 * Enhancement #2515: Enabling tracing by default if not explicitly disabled

   https://github.com/cs3org/reva/pull/2515

 * Enhancement #160: Implement the CS3 Lock API in the EOS storage driver

   https://github.com/cs3org/cs3apis/pull/160
   https://github.com/cs3org/reva/pull/2444

 * Enhancement #2686: Features for favorites xattrs in EOS, cache for scope expansion

   https://github.com/cs3org/reva/pull/2686

 * Enhancement #2494: Use sys ACLs for file permissions

   https://github.com/cs3org/reva/pull/2494

 * Enhancement #2522: Introduce events

   This will introduce events into the system. Events are a simple way to bring information from
   one service to another. Read `pkg/events/example` and subfolders for more information

   https://github.com/cs3org/reva/pull/2522

 * Enhancement #2685: Enable federated account access

   https://github.com/cs3org/reva/pull/2685

 * Enhancement #2801: Use functional options for client gRPC connections

   This will add more ability to configure the client side gRPC connections.

   https://github.com/cs3org/reva/pull/2801

 * Enhancement #2921: Use standard header for checksums

   On HEAD requests, we currently expose checksums (when available) using the
   ownCloud-specific header, which is typically consumed by the sync clients.

   This patch adds the standard Digest header using the standard format detailed at
   https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Digest. This is e.g. used
   by GFAL/Rucio clients in the context of managed transfers of datasets.

   https://github.com/cs3org/reva/pull/2921

 * Enhancement #2480: Group based capabilities

   We can now return specific capabilities for users who belong to certain configured groups.

   https://github.com/cs3org/reva/pull/2480

 * Enhancement #1787: Add support for HTTP TPC

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

 * Enhancement #2560: Mentix PromSD extensions

   The Mentix Prometheus SD scrape targets are now split into one file per service type, making
   health checks configuration easier. Furthermore, the local file connector for mesh data and
   the site registration endpoint have been dropped, as they aren't needed anymore.

   https://github.com/cs3org/reva/pull/2560

 * Enhancement #2613: Externalize custom mime types configuration for storage providers

   Added ability to configure custom mime types in an external JSON file, such that it can be reused
   when many storage providers are deployed at the same time.

   https://github.com/cs3org/reva/pull/2613

 * Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

 * Enhancement #2696: Preferences driver refactor and cbox sql implementation

   This PR uses the updated CS3APIs which accepts a namespace in addition to a single string key to
   recognize a user preference. It also refactors the GRPC service to support multiple drivers
   and adds the cbox SQL implementation.

   https://github.com/cs3org/reva/pull/2696

 * Enhancement #2052: New CS3API datatx methods

   CS3 datatx pull model methods: PullTransfer, RetryTransfer, ListTransfers Method
   CreateTransfer removed.

   https://github.com/cs3org/reva/pull/2052

 * Enhancement #2738: Site accounts site-global settings

   This PR extends the site accounts service by adding site-global settings. These are used to
   store test user credentials that are in return used by our BBE port to perform CS3API-specific
   health checks.

   https://github.com/cs3org/reva/pull/2738

 * Enhancement #2672: Further Site Accounts improvements

   Yet another PR to update the site accounts (and Mentix): New default site ID; Include service
   type in alerts; Naming unified; Remove obsolete stuff.

   https://github.com/cs3org/reva/pull/2672

 * Enhancement #2549: Site accounts improvements

   This PR improves the site accounts: - Removed/hid API key stuff - Added quick links to the main
   panel - Made alert notifications mandatory

   https://github.com/cs3org/reva/pull/2549

 * Enhancement #2488: Cephfs support keyrings with IDs

   https://github.com/cs3org/reva/pull/2488

 * Enhancement #2514: Reuse ocs role objects in other drivers

   https://github.com/cs3org/reva/pull/2514

 * Enhancement #2752: Refactor the rest user and group provider drivers

   We now maintain our own cache for all user and group data, and periodically refresh it. A redis
   server now becomes a necessary dependency, whereas it was optional previously.

   https://github.com/cs3org/reva/pull/2752

 * Enhancement #2946: Make user share indicators read from the share provider service

   https://github.com/cs3org/reva/pull/2946


Changelog for reva 1.18.0 (2022-02-11)
=======================================

The following sections list the changes in reva 1.18.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2370: Fixes for apps in public shares, project spaces for EOS driver
 * Fix #2374: Fix webdav copy of zero byte files
 * Fix #2478: Use ocs permission objects in the reva GRPC client
 * Fix #2368: Return wrapped paths for recycled items in storage provider
 * Chg #2354: Return not found when updating non existent space
 * Enh #1209: Reva CephFS module v0.2.1
 * Enh #2341: Use CS3 permissions API
 * Enh #2350: Add file locking methods to the storage and filesystem interfaces
 * Enh #2379: Add new file url of the app provider to the ocs capabilities
 * Enh #2369: Implement TouchFile from the CS3apis
 * Enh #2385: Allow to create new files with the app provider on public links
 * Enh #2397: Product field in OCS version
 * Enh #2393: Update tus/tusd to version 1.8.0
 * Enh #2205: Modify group and user managers to skip fetching specified metadata
 * Enh #2232: Make ocs resource info cache interoperable across drivers
 * Enh #2278: OIDC driver changes for lightweight users

Details
-------

 * Bugfix #2370: Fixes for apps in public shares, project spaces for EOS driver

   https://github.com/cs3org/reva/pull/2370

 * Bugfix #2374: Fix webdav copy of zero byte files

   We've fixed the webdav copy action of zero byte files, which was not performed because the
   webdav api assumed, that zero byte uploads are created when initiating the upload, which was
   recently removed from all storage drivers. Therefore the webdav api also uploads zero byte
   files after initiating the upload.

   https://github.com/cs3org/reva/pull/2374
   https://github.com/cs3org/reva/pull/2309

 * Bugfix #2478: Use ocs permission objects in the reva GRPC client

   There was a bug introduced by differing CS3APIs permission definitions for the same role
   across services. This is a first step in making all services use consistent definitions.

   https://github.com/cs3org/reva/pull/2478

 * Bugfix #2368: Return wrapped paths for recycled items in storage provider

   https://github.com/cs3org/reva/pull/2368

 * Change #2354: Return not found when updating non existent space

   If a spaceid of a space which is updated doesn't exist, handle it as a not found error.

   https://github.com/cs3org/reva/pull/2354

 * Enhancement #1209: Reva CephFS module v0.2.1

   https://github.com/cs3org/reva/pull/1209

 * Enhancement #2341: Use CS3 permissions API

   Added calls to the CS3 permissions API to the decomposedfs in order to check the user
   permissions.

   https://github.com/cs3org/reva/pull/2341

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

 * Enhancement #2205: Modify group and user managers to skip fetching specified metadata

   https://github.com/cs3org/reva/pull/2205

 * Enhancement #2232: Make ocs resource info cache interoperable across drivers

   https://github.com/cs3org/reva/pull/2232

 * Enhancement #2278: OIDC driver changes for lightweight users

   https://github.com/cs3org/reva/pull/2278


Changelog for reva 1.17.0 (2021-12-09)
=======================================

The following sections list the changes in reva 1.17.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2305: Make sure /app/new takes `target` as absolute path
 * Fix #2303: Fix content disposition header for public links files
 * Fix #2316: Fix the share types in propfinds
 * Fix #2803: Fix app provider for editor public links
 * Fix #2298: Remove share refs from trashbin
 * Fix #2309: Remove early finish for zero byte file uploads
 * Fix #1941: Fix TUS uploads with transfer token only
 * Chg #2210: Fix app provider new file creation and improved error codes
 * Enh #2217: OIDC auth driver for ESCAPE IAM
 * Enh #2256: Return user type in the response of the ocs GET user call
 * Enh #2315: Add new attributes to public link propfinds
 * Enh #2740: Implement space membership endpoints
 * Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
 * Enh #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

Details
-------

 * Bugfix #2305: Make sure /app/new takes `target` as absolute path

   A mini-PR to make the `target` parameter absolute (by prepending `/` if missing).

   https://github.com/cs3org/reva/pull/2305

 * Bugfix #2303: Fix content disposition header for public links files

   https://github.com/cs3org/reva/pull/2303
   https://github.com/cs3org/reva/pull/2297
   https://github.com/cs3org/reva/pull/2332
   https://github.com/cs3org/reva/pull/2346

 * Bugfix #2316: Fix the share types in propfinds

   The share types for public links were not correctly added to propfinds.

   https://github.com/cs3org/reva/pull/2316

 * Bugfix #2803: Fix app provider for editor public links

   Fixed opening the app provider in public links with the editor permission. The app provider
   failed to open the file in read write mode.

   https://github.com/owncloud/ocis/issues/2803
   https://github.com/cs3org/reva/pull/2310

 * Bugfix #2298: Remove share refs from trashbin

   https://github.com/cs3org/reva/pull/2298

 * Bugfix #2309: Remove early finish for zero byte file uploads

   We've fixed the upload of zero byte files by removing the early upload finishing mechanism.

   https://github.com/cs3org/reva/issues/2309
   https://github.com/owncloud/ocis/issues/2609

 * Bugfix #1941: Fix TUS uploads with transfer token only

   TUS uploads had been stopped when the user JWT token expired, even if only the transfer token
   should be validated. Now uploads will continue as intended.

   https://github.com/cs3org/reva/pull/1941

 * Change #2210: Fix app provider new file creation and improved error codes

   We've fixed the behavior for the app provider when creating new files. Previously the app
   provider would overwrite already existing files when creating a new file, this is now handled
   and prevented. The new file endpoint accepted a path to a file, but this does not work for spaces.
   Therefore we now use the resource id of the folder where the file should be created and a filename
   to create the new file. Also the app provider returns more useful error codes in a lot of cases.

   https://github.com/cs3org/reva/pull/2210

 * Enhancement #2217: OIDC auth driver for ESCAPE IAM

   This enhancement allows for oidc token authentication via the ESCAPE IAM service.
   Authentication relies on mappings of ESCAPE IAM groups to REVA users. For a valid token, if at
   the most one group from the groups claim is mapped to one REVA user, authentication can take
   place.

   https://github.com/cs3org/reva/pull/2217

 * Enhancement #2256: Return user type in the response of the ocs GET user call

   https://github.com/cs3org/reva/pull/2256

 * Enhancement #2315: Add new attributes to public link propfinds

   Added a new property "oc:signature-auth" to public link propfinds. This is a necessary change
   to be able to support archive downloads in password protected public links.

   https://github.com/cs3org/reva/pull/2315

 * Enhancement #2740: Implement space membership endpoints

   Implemented endpoints to add and remove members to spaces.

   https://github.com/owncloud/ocis/issues/2740
   https://github.com/cs3org/reva/pull/2250

 * Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

 * Enhancement #2314: OIDC: fallback if IDP doesn't provide "preferred_username" claim

   Some IDPs don't support the "preferred_username" claim. Fallback to the "email" claim in that
   case.

   https://github.com/cs3org/reva/pull/2314


Changelog for reva 1.16.0 (2021-11-19)
=======================================

The following sections list the changes in reva 1.16.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2245: Don't announce search-files capability
 * Fix #2247: Merge user ACLs from EOS to sys ACLs
 * Fix #2279: Return the inode of the version folder for files when listing in EOS
 * Fix #2294: Fix HTTP return code when path is invalid
 * Fix #2231: Fix share permission on a single file in sql share driver (cbox pkg)
 * Fix #2230: Fix open by default app and expose default app
 * Fix #2265: Fix nil pointer exception when resolving members of a group (rest driver)
 * Fix #1214: Fix restoring versions
 * Fix #2254: Fix spaces propfind
 * Fix #2260: Fix unset quota xattr on darwin
 * Fix #5776: Enforce permissions in public share apps
 * Fix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing
 * Fix #2287: Add public link access via mount-ID:token/relative-path to the scope
 * Fix #2244: Fix the permissions response for shared files in the cbox sql driver
 * Enh #2219: Add virtual view tests
 * Enh #2230: Add priority to app providers
 * Enh #2258: Improved error messages from the AppProviders
 * Enh #2119: Add authprovider owncloudsql
 * Enh #2211: Enhance the cbox share sql driver to store accepted group shares
 * Enh #2212: Filter root path according to the agent that makes the request
 * Enh #2237: Skip get user call in eosfs in case previous ones also failed
 * Enh #2266: Callback for the EOS UID cache to retry fetch for failed keys
 * Enh #2215: Aggregrate resource info properties for virtual views
 * Enh #2271: Revamp the favorite manager and add the cbox sql driver
 * Enh #2248: Cache whether a user home was created or not
 * Enh #2282: Return a proper NOT_FOUND error when a user or group is not found
 * Enh #2268: Add the reverseproxy http service
 * Enh #2207: Enable users to list all spaces
 * Enh #2286: Add trace ID to middleware loggers
 * Enh #2251: Mentix service inference
 * Enh #2218: Allow filtering of mime types supported by app providers
 * Enh #2213: Add public link share type to propfind response
 * Enh #2253: Support the file editor role for public links
 * Enh #2208: Reduce redundant stat calls when statting by resource ID
 * Enh #2235: Specify a list of allowed folders/files to be archived
 * Enh #2267: Restrict the paths where share creation is allowed
 * Enh #2252: Add the xattr sys.acl to SysACL (eosgrpc)
 * Enh #2239: Update toml configs

Details
-------

 * Bugfix #2245: Don't announce search-files capability

   The `dav.reports` capability contained a `search-files` report which is currently not
   implemented. We removed it from the defaults.

   https://github.com/cs3org/reva/pull/2245

 * Bugfix #2247: Merge user ACLs from EOS to sys ACLs

   https://github.com/cs3org/reva/pull/2247

 * Bugfix #2279: Return the inode of the version folder for files when listing in EOS

   https://github.com/cs3org/reva/pull/2279

 * Bugfix #2294: Fix HTTP return code when path is invalid

   Before when a path was invalid, the archiver returned a 500 error code. Now this is fixed and
   returns a 404 code.

   https://github.com/cs3org/reva/pull/2294

 * Bugfix #2231: Fix share permission on a single file in sql share driver (cbox pkg)

   https://github.com/cs3org/reva/pull/2231

 * Bugfix #2230: Fix open by default app and expose default app

   We've fixed the open by default app name behaviour which previously only worked, if the default
   app was configured by the provider address. We also now expose the default app on the
   `/app/list` endpoint to clients.

   https://github.com/cs3org/reva/issues/2230
   https://github.com/cs3org/cs3apis/pull/157

 * Bugfix #2265: Fix nil pointer exception when resolving members of a group (rest driver)

   https://github.com/cs3org/reva/pull/2265

 * Bugfix #1214: Fix restoring versions

   Restoring a version would not remove that version from the version list. Now the behavior is
   compatible to ownCloud 10.

   https://github.com/owncloud/ocis/issues/1214
   https://github.com/cs3org/reva/pull/2270

 * Bugfix #2254: Fix spaces propfind

   Fixed the deep listing of spaces.

   https://github.com/cs3org/reva/pull/2254

 * Bugfix #2260: Fix unset quota xattr on darwin

   Unset quota attributes were creating errors in the logfile on darwin.

   https://github.com/cs3org/reva/pull/2260

 * Bugfix #5776: Enforce permissions in public share apps

   A receiver of a read-only public share could still edit files via apps like Collabora. These
   changes enforce the share permissions in apps used on publicly shared resources.

   https://github.com/owncloud/web/issues/5776
   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/22142214

 * Bugfix #2767: Fix status code for WebDAV mkcol requests where an ancestor is missing

   We've fixed the status code to 409 according to the WebDAV standard for MKCOL requests where an
   ancestor is missing. Previously these requests would fail with an different error code (eg.
   500) because of storage driver limitations (eg. oCIS FS cannot handle recursive creation of
   directories).

   https://github.com/owncloud/ocis/issues/2767
   https://github.com/cs3org/reva/pull/2293

 * Bugfix #2287: Add public link access via mount-ID:token/relative-path to the scope

   https://github.com/cs3org/reva/pull/2287

 * Bugfix #2244: Fix the permissions response for shared files in the cbox sql driver

   https://github.com/cs3org/reva/pull/2244

 * Enhancement #2219: Add virtual view tests

   https://github.com/cs3org/reva/pull/2219

 * Enhancement #2230: Add priority to app providers

   Before the order of the list returned by the method FindProviders of app providers depended
   from the order in which the app provider registered themselves. Now, it is possible to specify a
   priority for each app provider, and even if an app provider re-register itself (for example
   after a restart), the order is kept.

   https://github.com/cs3org/reva/pull/2230
   https://github.com/cs3org/cs3apis/pull/157
   https://github.com/cs3org/reva/pull/2263

 * Enhancement #2258: Improved error messages from the AppProviders

   Some rather cryptic messages are now hidden to users, and some others are made more
   user-friendly. Support for multiple locales is still missing and out of scope for now.

   https://github.com/cs3org/reva/pull/2258

 * Enhancement #2119: Add authprovider owncloudsql

   We added an authprovider that can be configured to authenticate against an owncloud classic
   mysql database. It verifies the password from the oc_users table.

   https://github.com/cs3org/reva/pull/2119

 * Enhancement #2211: Enhance the cbox share sql driver to store accepted group shares

   https://github.com/cs3org/reva/pull/2211

 * Enhancement #2212: Filter root path according to the agent that makes the request

   https://github.com/cs3org/reva/pull/2212

 * Enhancement #2237: Skip get user call in eosfs in case previous ones also failed

   https://github.com/cs3org/reva/pull/2237

 * Enhancement #2266: Callback for the EOS UID cache to retry fetch for failed keys

   https://github.com/cs3org/reva/pull/2266

 * Enhancement #2215: Aggregrate resource info properties for virtual views

   https://github.com/cs3org/reva/pull/2215

 * Enhancement #2271: Revamp the favorite manager and add the cbox sql driver

   https://github.com/cs3org/reva/pull/2271

 * Enhancement #2248: Cache whether a user home was created or not

   Previously, on every call, we used to stat the user home to make sure that it existed. Now we cache
   it for a given amount of time so as to avoid repeated calls.

   https://github.com/cs3org/reva/pull/2248

 * Enhancement #2282: Return a proper NOT_FOUND error when a user or group is not found

   https://github.com/cs3org/reva/pull/2282

 * Enhancement #2268: Add the reverseproxy http service

   This PR adds an HTTP service which does the job of authenticating incoming requests via the reva
   middleware before forwarding them to the respective backends. This is useful for extensions
   which do not have the auth mechanisms.

   https://github.com/cs3org/reva/pull/2268

 * Enhancement #2207: Enable users to list all spaces

   Added a permission check if the user has the `list-all-spaces` permission. This enables users
   to list all spaces, even those which they are not members of.

   https://github.com/cs3org/reva/pull/2207

 * Enhancement #2286: Add trace ID to middleware loggers

   https://github.com/cs3org/reva/pull/2286

 * Enhancement #2251: Mentix service inference

   Previously, 4 different services per site had to be created in the GOCDB. This PR removes this
   redundancy by infering all endpoints from a single service entity, making site
   administration a lot easier.

   https://github.com/cs3org/reva/pull/2251

 * Enhancement #2218: Allow filtering of mime types supported by app providers

   https://github.com/cs3org/reva/pull/2218

 * Enhancement #2213: Add public link share type to propfind response

   Added share type for public links to propfind responses.

   https://github.com/cs3org/reva/pull/2213
   https://github.com/cs3org/reva/pull/2257

 * Enhancement #2253: Support the file editor role for public links

   https://github.com/cs3org/reva/pull/2253

 * Enhancement #2208: Reduce redundant stat calls when statting by resource ID

   https://github.com/cs3org/reva/pull/2208

 * Enhancement #2235: Specify a list of allowed folders/files to be archived

   Adds a configuration to the archiver service in order to specify a list of folders (as regex)
   that can be archived.

   https://github.com/cs3org/reva/pull/2235

 * Enhancement #2267: Restrict the paths where share creation is allowed

   This PR limits share creation to certain specified paths. These can be useful when users have
   access to global spaces and virtual views but these should not be sharable.

   https://github.com/cs3org/reva/pull/2267

 * Enhancement #2252: Add the xattr sys.acl to SysACL (eosgrpc)

   https://github.com/cs3org/reva/pull/2252

 * Enhancement #2239: Update toml configs

   We updated the local and drone configurations, cleanad up the example configs and removed the
   reva gen subcommand which was generating outdated config.

   https://github.com/cs3org/reva/pull/2239


Changelog for reva 1.15.0 (2021-10-26)
=======================================

The following sections list the changes in reva 1.15.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #2168: Override provider if was previously registered
 * Fix #2173: Fix archiver max size reached error
 * Fix #2167: Handle nil quota in decomposedfs
 * Fix #2153: Restrict EOS project spaces sharing permissions to admins and writers
 * Fix #2179: Fix the returned permissions for webdav uploads
 * Fix #2177: Retrieve the full path of a share when setting as
 * Chg #2479: Make apps able to work with public shares
 * Enh #2203: Add alerting webhook to SiteAcc service
 * Enh #2190: Update CODEOWNERS
 * Enh #2174: Inherit ACLs for files from parent directories
 * Enh #2152: Add a reference parameter to the getQuota request
 * Enh #2171: Add optional claim parameter to machine auth
 * Enh #2163: Nextcloud-based share manager for pkg/ocm/share
 * Enh #2135: Nextcloud test improvements
 * Enh #2180: Remove OCDAV options namespace parameter
 * Enh #2117: Add ocs cache warmup strategy for first request from the user
 * Enh #2170: Handle propfind requests for existing files
 * Enh #2165: Allow access to recycle bin for arbitrary paths outside homes
 * Enh #2193: Filter root paths according to user agent
 * Enh #2162: Implement the UpdateStorageSpace method
 * Enh #2189: Add user setting capability

Details
-------

 * Bugfix #2168: Override provider if was previously registered

   Previously if an AppProvider registered himself two times, for example after a failure, the
   mime types supported by the provider contained multiple times the same provider. Now this has
   been fixed, overriding the previous one.

   https://github.com/cs3org/reva/pull/2168

 * Bugfix #2173: Fix archiver max size reached error

   Previously in the total size count of the files being archived, the folders were taken into
   account, and this could cause a false max size reached error because the size of a directory is
   recursive-computed, causing the archive to be truncated. Now in the size count, the
   directories are skipped.

   https://github.com/cs3org/reva/pull/2173

 * Bugfix #2167: Handle nil quota in decomposedfs

   Do not nil pointer derefenrence when sending nil quota to decomposedfs

   https://github.com/cs3org/reva/issues/2167

 * Bugfix #2153: Restrict EOS project spaces sharing permissions to admins and writers

   https://github.com/cs3org/reva/pull/2153

 * Bugfix #2179: Fix the returned permissions for webdav uploads

   We've fixed the returned permissions for webdav uploads. It did not consider shares and public
   links for the permission calculation, but does so now.

   https://github.com/cs3org/reva/pull/2179
   https://github.com/cs3org/reva/pull/2151

 * Bugfix #2177: Retrieve the full path of a share when setting as

   Accepted or on shared by me

   https://github.com/cs3org/reva/pull/2177

 * Change #2479: Make apps able to work with public shares

   Public share receivers were not possible to use apps in public shares because the apps couldn't
   load the files in the public shares. This has now been made possible by changing the scope checks
   for public shares.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2143

 * Enhancement #2203: Add alerting webhook to SiteAcc service

   To integrate email alerting with the monitoring pipeline, a Prometheus webhook has been added
   to the SiteAcc service. Furthermore account settings have been extended/modified
   accordingly.

   https://github.com/cs3org/reva/pull/2203

 * Enhancement #2190: Update CODEOWNERS

   https://github.com/cs3org/reva/pull/2190

 * Enhancement #2174: Inherit ACLs for files from parent directories

   https://github.com/cs3org/reva/pull/2174

 * Enhancement #2152: Add a reference parameter to the getQuota request

   Implementation of [cs3org/cs3apis#147](https://github.com/cs3org/cs3apis/pull/147)

   Make the cs3apis accept a Reference in the getQuota Request to limit the call to a specific
   storage space.

   https://github.com/cs3org/reva/pull/2152
   https://github.com/cs3org/reva/pull/2178
   https://github.com/cs3org/reva/pull/2187

 * Enhancement #2171: Add optional claim parameter to machine auth

   https://github.com/cs3org/reva/issues/2171
   https://github.com/cs3org/reva/pull/2176

 * Enhancement #2163: Nextcloud-based share manager for pkg/ocm/share

   Note that pkg/ocm/share is very similar to pkg/share, but it deals with cs3/sharing/ocm
   whereas pkg/share deals with cs3/sharing/collaboration

   https://github.com/cs3org/reva/pull/2163

 * Enhancement #2135: Nextcloud test improvements

   https://github.com/cs3org/reva/pull/2135

 * Enhancement #2180: Remove OCDAV options namespace parameter

   We dropped the namespace parameter, as it is not used in the options handler.

   https://github.com/cs3org/reva/pull/2180

 * Enhancement #2117: Add ocs cache warmup strategy for first request from the user

   https://github.com/cs3org/reva/pull/2117

 * Enhancement #2170: Handle propfind requests for existing files

   https://github.com/cs3org/reva/pull/2170

 * Enhancement #2165: Allow access to recycle bin for arbitrary paths outside homes

   https://github.com/cs3org/reva/pull/2165
   https://github.com/cs3org/reva/pull/2188

 * Enhancement #2193: Filter root paths according to user agent

   Adds a new rule setting in the storage registry ("allowed_user_agents"), that allows a user to
   specify which storage provider shows according to the user agent that made the request.

   https://github.com/cs3org/reva/pull/2193

 * Enhancement #2162: Implement the UpdateStorageSpace method

   Added the UpdateStorageSpace method to the decomposedfs.

   https://github.com/cs3org/reva/pull/2162
   https://github.com/cs3org/reva/pull/2195
   https://github.com/cs3org/reva/pull/2196

 * Enhancement #2189: Add user setting capability

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

 * Fix #2103: AppProvider: propagate back errors reported by WOPI
 * Fix #2149: Remove excess info from the http list app providers endpoint
 * Fix #2114: Add as default app while registering and skip unset mimetypes
 * Fix #2095: Fix app open when multiple app providers are present
 * Fix #2135: Make TUS capabilities configurable
 * Fix #2076: Fix chi routing
 * Fix #2077: Fix concurrent registration of mimetypes
 * Fix #2154: Return OK when trying to delete a non existing reference
 * Fix #2078: Fix nil pointer exception in stat
 * Fix #2073: Fix opening a readonly filetype with WOPI
 * Fix #2140: Map GRPC error codes to REVA errors
 * Fix #2147: Follow up of #2138: this is the new expected format
 * Fix #2116: Differentiate share types when retrieving received shares in sql driver
 * Fix #2074: Fix Stat() for EOS storage provider
 * Fix #2151: Fix return code for webdav uploads when the token expired
 * Chg #2121: Sharemanager API change
 * Enh #2090: Return space name during list storage spaces
 * Enh #2138: Default AppProvider on top of the providers list
 * Enh #2137: Revamp app registry and add parameter to control file creation
 * Enh #145: UI improvements for the AppProviders
 * Enh #2088: Add archiver and app provider to ocs capabilities
 * Enh #2537: Add maximum files and size to archiver capabilities
 * Enh #2100: Add support for resource id to the archiver
 * Enh #2158: Augment the Id of new spaces
 * Enh #2085: Make encoding user groups in access tokens configurable
 * Enh #146: Filter the denial shares (permission = 0) out of
 * Enh #2141: Use golang v1.17
 * Enh #2053: Safer defaults for TLS verification on LDAP connections
 * Enh #2115: Reduce code duplication in LDAP related drivers
 * Enh #1989: Add redirects from OC10 URL formats
 * Enh #2479: Limit publicshare and resourceinfo scope content
 * Enh #2071: Implement listing favorites via the dav report API
 * Enh #2091: Nextcloud share managers
 * Enh #2070: More unit tests for the Nextcloud storage provider
 * Enh #2087: More unit tests for the Nextcloud auth and user managers
 * Enh #2075: Make owncloudsql leverage existing filecache index
 * Enh #2050: Add a share types filter to the OCS API
 * Enh #2134: Use space Type from request
 * Enh #2132: Align local tests with drone setup
 * Enh #2095: Whitelisting for apps
 * Enh #2155: Pass an extra query parameter to WOPI /openinapp with a

Details
-------

 * Bugfix #2103: AppProvider: propagate back errors reported by WOPI

   On /app/open and return base64-encoded fileids on /app/new

   https://github.com/cs3org/reva/pull/2103

 * Bugfix #2149: Remove excess info from the http list app providers endpoint

   We've removed excess info from the http list app providers endpoint. The app provider section
   contained all mime types supported by a certain app provider, which led to a very big JSON
   payload and since they are not used they have been removed again. Mime types not on the mime type
   configuration list always had `application/octet-stream` as a file extension and
   `APPLICATION/OCTET-STREAM file` as name and description. Now these information are just
   omitted.

   https://github.com/cs3org/reva/pull/2149
   https://github.com/owncloud/ocis/pull/2603
   https://github.com/cs3org/reva/pull/2138

 * Bugfix #2114: Add as default app while registering and skip unset mimetypes

   We fixed that app providers will be set as default app while registering if configured. Also we
   changed the behaviour that listing supported mimetypes only displays allowed / configured
   mimetypes.

   https://github.com/cs3org/reva/pull/2114
   https://github.com/cs3org/reva/pull/2095

 * Bugfix #2095: Fix app open when multiple app providers are present

   We've fixed the gateway behavior, that when multiple app providers are present, it always
   returned that we have duplicate names for app providers. This was due the call to
   GetAllProviders() without any subsequent filtering by name. Now this filter mechanism is in
   place and the duplicate app providers error will only appear if a real duplicate is found.

   https://github.com/cs3org/reva/issues/2095
   https://github.com/cs3org/reva/pull/2117

 * Bugfix #2135: Make TUS capabilities configurable

   We've fixed the configuration for the TUS capabilities, which will now take the given
   configuration instead of always using hardcoded defaults.

   https://github.com/cs3org/reva/pull/2135

 * Bugfix #2076: Fix chi routing

   Chi routes based on the URL.RawPath, which is not updated by the shiftPath based routing used in
   reva. By setting the RawPath to an empty string chi will fall pack to URL.Path, allowing it to
   match percent encoded path segments, e.g. when trying to match emails or multibyte
   characters.

   https://github.com/cs3org/reva/pull/2076

 * Bugfix #2077: Fix concurrent registration of mimetypes

   We fixed registering mimetypes in the mime package when starting multiple storage providers
   in the same process.

   https://github.com/cs3org/reva/pull/2077

 * Bugfix #2154: Return OK when trying to delete a non existing reference

   When the gateway declines a share we can ignore a non existing reference.

   https://github.com/cs3org/reva/pull/2154
   https://github.com/owncloud/ocis/pull/2603

 * Bugfix #2078: Fix nil pointer exception in stat

   https://github.com/cs3org/reva/pull/2078

 * Bugfix #2073: Fix opening a readonly filetype with WOPI

   This change fixes the opening of filetypes that are only supported to be viewed and not to be
   edited by some WOPI compliant office suites.

   https://github.com/cs3org/reva/pull/2073

 * Bugfix #2140: Map GRPC error codes to REVA errors

   We've fixed the error return behaviour in the gateway which would return GRPC error codes from
   the auth middleware. Now it returns REVA errors which other parts of REVA are also able to
   understand.

   https://github.com/cs3org/reva/pull/2140

 * Bugfix #2147: Follow up of #2138: this is the new expected format

   For the mime types configuration for the AppRegistry.

   https://github.com/cs3org/reva/pull/2147

 * Bugfix #2116: Differentiate share types when retrieving received shares in sql driver

   https://github.com/cs3org/reva/pull/2116

 * Bugfix #2074: Fix Stat() for EOS storage provider

   This change fixes the convertion between the eosclient.FileInfo to ResourceInfo, in which
   the field ArbitraryMetadata was missing. Moreover, to be consistent with
   SetArbitraryMetadata() EOS implementation, all the "user." prefix are stripped out from the
   xattrs.

   https://github.com/cs3org/reva/pull/2074

 * Bugfix #2151: Fix return code for webdav uploads when the token expired

   We've fixed the behavior webdav uploads when the token expired before the final stat.
   Previously clients would receive a http 500 error which is wrong, because the file was
   successfully uploaded and only the stat couldn't be performed. Now we return a http 200 ok and
   the clients will fetch the file info in a separate propfind request.

   Also we introduced the upload expires header on the webdav/TUS and datagateway endpoints, to
   signal clients how long an upload can be performed.

   https://github.com/cs3org/reva/pull/2151

 * Change #2121: Sharemanager API change

   This PR updates reva to reflect the share manager CS3 API changes.

   https://github.com/cs3org/reva/pull/2121

 * Enhancement #2090: Return space name during list storage spaces

   In the decomposedfs we return now the space name in the response which is stored in the extended
   attributes.

   https://github.com/cs3org/reva/issues/2090

 * Enhancement #2138: Default AppProvider on top of the providers list

   For each mime type

   Now for each mime type, when asking for the list of mime types, the default AppProvider, set both
   using the config and the SetDefaultProviderForMimeType method, is always in the top of the
   list of AppProviders. The config for the Providers and Mime Types for the AppRegistry changed,
   using a list instead of a map. In fact the list of mime types returned by ListSupportedMimeTypes
   is now ordered according the config.

   https://github.com/cs3org/reva/pull/2138

 * Enhancement #2137: Revamp app registry and add parameter to control file creation

   https://github.com/cs3org/reva/pull/2137

 * Enhancement #145: UI improvements for the AppProviders

   Mime types and their friendly names are now handled in the /app/list HTTP endpoint, and an
   additional /app/new endpoint is made available to create new files for apps.

   https://github.com/cs3org/cs3apis/pull/145
   https://github.com/cs3org/reva/pull/2067

 * Enhancement #2088: Add archiver and app provider to ocs capabilities

   The archiver and app provider has been added to the ocs capabilities.

   https://github.com/cs3org/reva/pull/2088
   https://github.com/owncloud/ocis/pull/2529

 * Enhancement #2537: Add maximum files and size to archiver capabilities

   We added the maximum files count and maximum archive size of the archiver to the capabilities
   endpoint. Clients can use this to generate warnings before the actual archive creation fails.

   https://github.com/owncloud/ocis/issues/2537
   https://github.com/cs3org/reva/pull/2105

 * Enhancement #2100: Add support for resource id to the archiver

   Before the archiver only supported resources provided by a path. Now also the resources ID are
   supported in order to specify the content of the archive to download. The parameters accepted
   by the archiver are two: an optional list of `path` (containing the paths of the resources) and
   an optional list of `id` (containing the resources IDs of the resources).

   https://github.com/cs3org/reva/issues/2097
   https://github.com/cs3org/reva/pull/2100

 * Enhancement #2158: Augment the Id of new spaces

   Newly created spaces were missing the Root reference and the storage id in the space id.

   https://github.com/cs3org/reva/issues/2158

 * Enhancement #2085: Make encoding user groups in access tokens configurable

   https://github.com/cs3org/reva/pull/2085

 * Enhancement #146: Filter the denial shares (permission = 0) out of

   The Shared-with-me UI view. Also they work regardless whether they are accepted or not,
   therefore there's no point to expose them.

   https://github.com/cs3org/cs3apis/pull/146
   https://github.com/cs3org/reva/pull/2072

 * Enhancement #2141: Use golang v1.17

   https://github.com/cs3org/reva/pull/2141

 * Enhancement #2053: Safer defaults for TLS verification on LDAP connections

   The LDAP client connections were hardcoded to ignore certificate validation errors. Now
   verification is enabled by default and a new config parameter 'insecure' is introduced to
   override that default. It is also possible to add trusted Certificates by using the new
   'cacert' config paramter.

   https://github.com/cs3org/reva/pull/2053

 * Enhancement #2115: Reduce code duplication in LDAP related drivers

   https://github.com/cs3org/reva/pull/2115

 * Enhancement #1989: Add redirects from OC10 URL formats

   Added redirectors for ownCloud 10 URLs. This allows users to continue to use their bookmarks
   from ownCloud 10 in ocis.

   https://github.com/cs3org/reva/pull/1989

 * Enhancement #2479: Limit publicshare and resourceinfo scope content

   We changed the publicshare and resourceinfo scopes to contain only necessary values. This
   reduces the size of the resulting token and also limits the amount of data which can be leaked.

   https://github.com/owncloud/ocis/issues/2479
   https://github.com/cs3org/reva/pull/2093

 * Enhancement #2071: Implement listing favorites via the dav report API

   Added filter-files to the dav REPORT API. This enables the listing of favorites.

   https://github.com/cs3org/reva/pull/2071
   https://github.com/cs3org/reva/pull/2086

 * Enhancement #2091: Nextcloud share managers

   Share manager that uses Nextcloud as a backend

   https://github.com/cs3org/reva/pull/2091

 * Enhancement #2070: More unit tests for the Nextcloud storage provider

   Adds more unit tests for the Nextcloud storage provider.

   https://github.com/cs3org/reva/pull/2070

 * Enhancement #2087: More unit tests for the Nextcloud auth and user managers

   Adds more unit tests for the Nextcloud auth manager and the Nextcloud user manager

   https://github.com/cs3org/reva/pull/2087

 * Enhancement #2075: Make owncloudsql leverage existing filecache index

   When listing folders the SQL query now uses an existing index on the filecache table.

   https://github.com/cs3org/reva/pull/2075

 * Enhancement #2050: Add a share types filter to the OCS API

   Added a filter to the OCS API to filter the received shares by type.

   https://github.com/cs3org/reva/pull/2050

 * Enhancement #2134: Use space Type from request

   In the decomposedfs we now use the space type from the request when creating a new space.

   https://github.com/cs3org/reva/issues/2134

 * Enhancement #2132: Align local tests with drone setup

   We fixed running the tests locally and align it with the drone setup.

   https://github.com/cs3org/reva/issues/2132

 * Enhancement #2095: Whitelisting for apps

   AppProvider supported mime types are now overridden in its configuration. A friendly name, a
   description, an extension, an icon and a default app, can be configured in the AppRegistry for
   each mime type.

   https://github.com/cs3org/reva/pull/2095

 * Enhancement #2155: Pass an extra query parameter to WOPI /openinapp with a

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

 * Fix #2024: Fixes for http appprovider endpoints
 * Fix #2054: Fix the response after deleting a share
 * Fix #2026: Fix moving of a shared file
 * Fix #2047: Do not truncate logs on restart
 * Fix #1605: Allow to expose full paths in OCS API
 * Fix #2033: Fix the storage id of shares
 * Fix #2059: Remove "Got registration for user manager" print statements
 * Fix #2051: Remove malformed parameters from WOPI discovery URLs
 * Fix #2055: Fix uploads of empty files
 * Fix #1991: Remove share references when declining shares
 * Fix #2030: Fix superfluous WriteHeader on file upload
 * Enh #2034: Fail initialization of a WOPI AppProvider if
 * Enh #1968: Use a URL object in OpenInAppResponse
 * Enh #1698: Implement folder download as archive
 * Enh #2042: Escape ldap filters
 * Enh #2028: Machine auth provider
 * Enh #2043: Nextcloud user backend
 * Enh #2006: Move ocs API to go-chi/chi based URL routing
 * Enh #1994: Add owncloudsql driver for the userprovider
 * Enh #1971: Add documentation for runtime-plugins
 * Enh #2044: Add utility methods for creating share filters
 * Enh #2065: New sharing role Manager
 * Enh #2015: Add spaces to the list of capabilities
 * Enh #2041: Create operations for Spaces
 * Enh #2029: Tracing agent configuration

Details
-------

 * Bugfix #2024: Fixes for http appprovider endpoints

   https://github.com/cs3org/reva/pull/2024
   https://github.com/cs3org/reva/pull/1968

 * Bugfix #2054: Fix the response after deleting a share

   Added the deleted share to the response after deleting it.

   https://github.com/cs3org/reva/pull/2054

 * Bugfix #2026: Fix moving of a shared file

   As the share receiver, moving a shared file to another share was not possible.

   https://github.com/cs3org/reva/pull/2026

 * Bugfix #2047: Do not truncate logs on restart

   This change fixes the way log files were opened. Before they were truncated and now the log file
   will be open in append mode and created it if it does not exist.

   https://github.com/cs3org/reva/pull/2047

 * Bugfix #1605: Allow to expose full paths in OCS API

   Before this fix a share file_target was always harcoded to use a base path. This fix provides the
   possiblity to expose full paths in the OCIS API and asymptotically in OCIS web.

   https://github.com/cs3org/reva/pull/1605

 * Bugfix #2033: Fix the storage id of shares

   The storageid in the share object contained an incorrect value.

   https://github.com/cs3org/reva/pull/2033

 * Bugfix #2059: Remove "Got registration for user manager" print statements

   Removed the "Got registration for user manager" print statements which spams the log output
   without respecting any log level.

   https://github.com/cs3org/reva/pull/2059

 * Bugfix #2051: Remove malformed parameters from WOPI discovery URLs

   This change fixes the parsing of WOPI discovery URLs for MSOffice /hosting/discovery
   endpoint. This endpoint is known to contain malformed query paramters and therefore this fix
   removes them.

   https://github.com/cs3org/reva/pull/2051

 * Bugfix #2055: Fix uploads of empty files

   This change fixes upload of empty files. Previously this was broken and only worked for the
   owncloud filesystem as it bypasses the semantics of the InitiateFileUpload call to touch a
   local file.

   https://github.com/cs3org/reva/pull/2055

 * Bugfix #1991: Remove share references when declining shares

   Implemented the removal of share references when a share gets declined. Now when a user
   declines a share it will no longer be listed in their `Shares` directory.

   https://github.com/cs3org/reva/pull/1991

 * Bugfix #2030: Fix superfluous WriteHeader on file upload

   Removes superfluous Writeheader on file upload and therefore removes the error message
   "http: superfluous response.WriteHeader call from
   github.com/cs3org/reva/internal/http/interceptors/log.(*responseLogger).WriteHeader
   (log.go:154)"

   https://github.com/cs3org/reva/pull/2030

 * Enhancement #2034: Fail initialization of a WOPI AppProvider if

   The underlying app is not WOPI-compliant nor it is supported by the WOPI bridge extensions

   https://github.com/cs3org/reva/pull/2034

 * Enhancement #1968: Use a URL object in OpenInAppResponse

   https://github.com/cs3org/reva/pull/1968

 * Enhancement #1698: Implement folder download as archive

   Adds a new http service which will create an archive (platform dependent, zip in windows and tar
   in linux) given a list of file.

   https://github.com/cs3org/reva/issues/1698
   https://github.com/cs3org/reva/pull/2066

 * Enhancement #2042: Escape ldap filters

   Added ldap filter escaping to increase the security of reva.

   https://github.com/cs3org/reva/pull/2042

 * Enhancement #2028: Machine auth provider

   Adds a new authentication method used to impersonate users, using a shared secret, called
   api-key.

   https://github.com/cs3org/reva/pull/2028

 * Enhancement #2043: Nextcloud user backend

   Adds Nextcloud as a user backend (Nextcloud drivers for 'auth' and 'user'). Also adds back the
   Nextcloud storage integration tests.

   https://github.com/cs3org/reva/pull/2043

 * Enhancement #2006: Move ocs API to go-chi/chi based URL routing

   https://github.com/cs3org/reva/issues/1986
   https://github.com/cs3org/reva/pull/2006

 * Enhancement #1994: Add owncloudsql driver for the userprovider

   We added a new backend for the userprovider that is backed by an owncloud 10 database. By default
   the `user_id` column is used as the reva user username and reva user opaque id. When setting
   `join_username=true` the reva user username is joined from the `oc_preferences` table
   (`appid='core' AND configkey='username'`) instead. When setting
   `join_ownclouduuid=true` the reva user opaqueid is joined from the `oc_preferences` table
   (`appid='core' AND configkey='ownclouduuid'`) instead. This allows more flexible
   migration strategies. It also supports a `enable_medial_search` config option when
   searching users that will enclose the query with `%`.

   https://github.com/cs3org/reva/pull/1994

 * Enhancement #1971: Add documentation for runtime-plugins

   https://github.com/cs3org/reva/pull/1971

 * Enhancement #2044: Add utility methods for creating share filters

   Updated the CS3 API to include the new share grantee filter and added utility methods for
   creating share filters. This will help making the code more concise.

   https://github.com/cs3org/reva/pull/2044

 * Enhancement #2065: New sharing role Manager

   The new Manager role is equivalent to a Co-Owner with the difference that a Manager can create
   grants on the root of the Space. This means inviting a user to a space will not require an action
   from them, as the Manager assigns the grants.

   https://github.com/cs3org/reva/pull/2065

 * Enhancement #2015: Add spaces to the list of capabilities

   In order for clients to be aware of the new spaces feature we need to enable the `spaces` flag on
   the capabilities' endpoint.

   https://github.com/cs3org/reva/pull/2015

 * Enhancement #2041: Create operations for Spaces

   DecomposedFS is aware now of the concept of Spaces, and supports for creating them.

   https://github.com/cs3org/reva/pull/2041

 * Enhancement #2029: Tracing agent configuration

   Earlier we could only use the collector URL directly, but since an agent can be deployed as a
   sidecar process it makes much more sense to use it instead of the collector directly.

   https://github.com/cs3org/reva/pull/2029


Changelog for reva 1.12.0 (2021-08-24)
=======================================

The following sections list the changes in reva 1.12.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1819: Disable notifications
 * Fix #2000: Fix dependency on tests
 * Fix #1957: Fix etag propagation on deletes
 * Fix #1960: Return the updated share after updating
 * Fix #1993: Fix owncloudsql GetMD
 * Fix #1954: Fix response format of the sharees API
 * Fix #1965: Fix the file target of user and group shares
 * Fix #1956: Fix trashbin listing with depth 0
 * Fix #1987: Fix windows build
 * Fix #1990: Increase oc10 compatibility of owncloudsql
 * Fix #1978: Owner type is optional
 * Fix #1980: Propagate the etag after restoring a file version
 * Fix #1985: Add quota stubs
 * Fix #1992: Check if symlink exists instead of spamming the console
 * Fix #1913: Logic to restore files to readonly nodes
 * Chg #1982: Move user context methods into a separate `userctx` package
 * Enh #1946: Add share manager that connects to oc10 databases
 * Enh #1983: Add Codacy unit test coverage
 * Enh #1803: Introduce new webdav spaces endpoint
 * Enh #1998: Initial version of the Nextcloud storage driver
 * Enh #1984: Replace OpenCensus with OpenTelemetry
 * Enh #1861: Add support for runtime plugins
 * Enh #2008: Site account extensions

Details
-------

 * Bugfix #1819: Disable notifications

   The presence of the key `notifications` in the capabilities' response would cause clients to
   attempt to poll the notifications endpoint, which is not yet supported. To prevent the
   unnecessary bandwidth we are disabling this altogether.

   https://github.com/cs3org/reva/pull/1819

 * Bugfix #2000: Fix dependency on tests

   The Nextcloud storage driver depended on a mock http client from the tests/ folder This broke
   the Docker build The dependency was removed A check was added to test the Docker build on each PR

   https://github.com/cs3org/reva/pull/2000

 * Bugfix #1957: Fix etag propagation on deletes

   When deleting a file the etag propagation would skip the parent of the deleted file.

   https://github.com/cs3org/reva/pull/1957

 * Bugfix #1960: Return the updated share after updating

   When updating the state of a share in the in-memory share manager the old share state was
   returned instead of the updated state.

   https://github.com/cs3org/reva/pull/1960

 * Bugfix #1993: Fix owncloudsql GetMD

   The GetMD call internally was not prefixing the path when looking up resources by id.

   https://github.com/cs3org/reva/pull/1993

 * Bugfix #1954: Fix response format of the sharees API

   The sharees API wasn't returning the users and groups arrays correctly.

   https://github.com/cs3org/reva/pull/1954

 * Bugfix #1965: Fix the file target of user and group shares

   In some cases the file target of user and group shares was not properly prefixed.

   https://github.com/cs3org/reva/pull/1965
   https://github.com/cs3org/reva/pull/1967

 * Bugfix #1956: Fix trashbin listing with depth 0

   The trashbin API handled requests with depth 0 the same as request with a depth of 1.

   https://github.com/cs3org/reva/pull/1956

 * Bugfix #1987: Fix windows build

   Add the necessary `golang.org/x/sys/windows` package import to `owncloud` and
   `owncloudsql` storage drivers.

   https://github.com/cs3org/reva/pull/1987

 * Bugfix #1990: Increase oc10 compatibility of owncloudsql

   We added a few changes to the owncloudsql storage driver to behave more like oc10.

   https://github.com/cs3org/reva/pull/1990

 * Bugfix #1978: Owner type is optional

   When reading the user from the extended attributes the user type might not be set, in this case we
   now return a user with an invalid type, which correctly reflects the state on disk.

   https://github.com/cs3org/reva/pull/1978

 * Bugfix #1980: Propagate the etag after restoring a file version

   The decomposedfs didn't propagate after restoring a file version.

   https://github.com/cs3org/reva/pull/1980

 * Bugfix #1985: Add quota stubs

   The `owncloud` and `owncloudsql` drivers now read the available quota from disk to no longer
   always return 0, which causes the web UI to disable uploads.

   https://github.com/cs3org/reva/pull/1985

 * Bugfix #1992: Check if symlink exists instead of spamming the console

   The logs have been spammed with messages like `could not create symlink for ...` when using the
   decomposedfs, eg. with the oCIS storage. We now check if the link exists before trying to create
   it.

   https://github.com/cs3org/reva/pull/1992

 * Bugfix #1913: Logic to restore files to readonly nodes

   This impacts solely the DecomposedFS. Prior to these changes there was no validation when a
   user tried to restore a file from the trashbin to a share location (i.e any folder under
   `/Shares`).

   With this patch if the user restoring the resource has write permissions on the share, restore
   is possible.

   https://github.com/cs3org/reva/pull/1913

 * Change #1982: Move user context methods into a separate `userctx` package

   https://github.com/cs3org/reva/pull/1982

 * Enhancement #1946: Add share manager that connects to oc10 databases

   https://github.com/cs3org/reva/pull/1946

 * Enhancement #1983: Add Codacy unit test coverage

   This PR adds unit test coverage upload to Codacy.

   https://github.com/cs3org/reva/pull/1983

 * Enhancement #1803: Introduce new webdav spaces endpoint

   Clients can now use a new webdav endpoint
   `/dav/spaces/<storagespaceid>/relative/path/to/file` to directly access storage
   spaces.

   The `<storagespaceid>` can be retrieved using the ListStorageSpaces CS3 api call.

   https://github.com/cs3org/reva/pull/1803

 * Enhancement #1998: Initial version of the Nextcloud storage driver

   This is not usable yet in isolation, but it's a first component of
   https://github.com/pondersource/sciencemesh-nextcloud

   https://github.com/cs3org/reva/pull/1998

 * Enhancement #1984: Replace OpenCensus with OpenTelemetry

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

 * Enhancement #1861: Add support for runtime plugins

   This PR introduces a new plugin package, that allows loading external plugins into Reva at
   runtime. The hashicorp go-plugin framework was used to facilitate the plugin loading and
   communication.

   https://github.com/cs3org/reva/pull/1861

 * Enhancement #2008: Site account extensions

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

 * Fix #1899: Fix chunked uploads for new versions
 * Fix #1906: Fix copy over existing resource
 * Fix #1891: Delete Shared Resources as Receiver
 * Fix #1907: Error when creating folder with existing name
 * Fix #1937: Do not overwrite more specific matches when finding storage providers
 * Fix #1939: Fix the share jail permissions in the decomposedfs
 * Fix #1932: Numerous fixes to the owncloudsql storage driver
 * Fix #1912: Fix response when listing versions of another user
 * Fix #1910: Get user groups recursively in the cbox rest user driver
 * Fix #1904: Set Content-Length to 0 when swallowing body in the datagateway
 * Fix #1911: Fix version order in propfind responses
 * Fix #1926: Trash Bin in oCIS Storage Operations
 * Fix #1901: Fix response code when folder doesnt exist on upload
 * Enh #1785: Extend app registry with AddProvider method and mimetype filters
 * Enh #1938: Add methods to get and put context values
 * Enh #1798: Add support for a deny-all permission on references
 * Enh #1916: Generate updated protobuf bindings for EOS GRPC
 * Enh #1887: Add "a" and "l" filter for grappa queries
 * Enh #1919: Run gofmt before building
 * Enh #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)
 * Enh #1944: Implement listing supported mime types in app registry
 * Enh #1870: Be defensive about wrongly quoted etags
 * Enh #1940: Reduce memory usage when uploading with S3ng storage
 * Enh #1888: Refactoring of the webdav code
 * Enh #1900: Check for illegal names while uploading or moving files
 * Enh #1925: Refactor listing and statting across providers for virtual views

Details
-------

 * Bugfix #1899: Fix chunked uploads for new versions

   Chunked uploads didn't create a new version, when the file to upload already existed.

   https://github.com/cs3org/reva/pull/1899

 * Bugfix #1906: Fix copy over existing resource

   When the target of a copy already exists, the existing resource will be moved to the trashbin
   before executing the copy.

   https://github.com/cs3org/reva/pull/1906

 * Bugfix #1891: Delete Shared Resources as Receiver

   It is now possible to delete a shared resource as a receiver and not having the data ending up in
   the receiver's trash bin, causing a possible leak.

   https://github.com/cs3org/reva/pull/1891

 * Bugfix #1907: Error when creating folder with existing name

   When a user tried to create a folder with the name of an existing file or folder the service didn't
   return a response body containing the error.

   https://github.com/cs3org/reva/pull/1907

 * Bugfix #1937: Do not overwrite more specific matches when finding storage providers

   Depending on the order of rules in the registry it could happend that more specific matches
   (e.g. /home/Shares) were overwritten by more general ones (e.g. /home). This PR makes sure
   that the registry always returns the most specific match.

   https://github.com/cs3org/reva/pull/1937

 * Bugfix #1939: Fix the share jail permissions in the decomposedfs

   The share jail should be not writable

   https://github.com/cs3org/reva/pull/1939

 * Bugfix #1932: Numerous fixes to the owncloudsql storage driver

   The owncloudsql storage driver received numerous bugfixes and cleanups.

   https://github.com/cs3org/reva/pull/1932

 * Bugfix #1912: Fix response when listing versions of another user

   The OCS API returned the wrong response when a user tried to list the versions of another user's
   file.

   https://github.com/cs3org/reva/pull/1912

 * Bugfix #1910: Get user groups recursively in the cbox rest user driver

   https://github.com/cs3org/reva/pull/1910

 * Bugfix #1904: Set Content-Length to 0 when swallowing body in the datagateway

   When swallowing the body the Content-Lenght needs to be set to 0 to prevent proxies from reading
   the body.

   https://github.com/cs3org/reva/pull/1904

 * Bugfix #1911: Fix version order in propfind responses

   The order of the file versions in propfind responses was incorrect.

   https://github.com/cs3org/reva/pull/1911

 * Bugfix #1926: Trash Bin in oCIS Storage Operations

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

 * Bugfix #1901: Fix response code when folder doesnt exist on upload

   When a new file was uploaded to a non existent folder the response code was incorrect.

   https://github.com/cs3org/reva/pull/1901

 * Enhancement #1785: Extend app registry with AddProvider method and mimetype filters

   https://github.com/cs3org/reva/issues/1779
   https://github.com/cs3org/reva/pull/1785
   https://github.com/cs3org/cs3apis/pull/131

 * Enhancement #1938: Add methods to get and put context values

   Added `GetKeyValues` and `PutKeyValues` methods to fetch/put values from/to context.

   https://github.com/cs3org/reva/pull/1938

 * Enhancement #1798: Add support for a deny-all permission on references

   And implement it on the EOS storage

   http://github.com/cs3org/reva/pull/1798

 * Enhancement #1916: Generate updated protobuf bindings for EOS GRPC

   https://github.com/cs3org/reva/pull/1916

 * Enhancement #1887: Add "a" and "l" filter for grappa queries

   This PR adds the namespace filters "a" and "l" for grappa queries. With no filter will look into
   primary and e-groups, with "a" will look into primary/secondary/service/e-groups and with
   "l" will look into lightweight accounts.

   https://github.com/cs3org/reva/issues/1773
   https://github.com/cs3org/reva/pull/1887

 * Enhancement #1919: Run gofmt before building

   https://github.com/cs3org/reva/pull/1919

 * Enhancement #1927: Implement RollbackToVersion for eosgrpc (needs a newer EOS MGM)

   https://github.com/cs3org/reva/pull/1927

 * Enhancement #1944: Implement listing supported mime types in app registry

   https://github.com/cs3org/reva/pull/1944

 * Enhancement #1870: Be defensive about wrongly quoted etags

   When ocdav renders etags it will now try to correct them to the definition as *quoted strings*
   which do not contain `"`. This prevents double or triple quoted etags on the webdav api.

   https://github.com/cs3org/reva/pull/1870

 * Enhancement #1940: Reduce memory usage when uploading with S3ng storage

   The memory usage could be high when uploading files using the S3ng storage. By providing the
   actual file size when triggering `PutObject`, the overall memory usage is reduced.

   https://github.com/cs3org/reva/pull/1940

 * Enhancement #1888: Refactoring of the webdav code

   Refactored the webdav code to make it reusable.

   https://github.com/cs3org/reva/pull/1888

 * Enhancement #1900: Check for illegal names while uploading or moving files

   The code was not checking for invalid file names during uploads and moves.

   https://github.com/cs3org/reva/pull/1900

 * Enhancement #1925: Refactor listing and statting across providers for virtual views

   https://github.com/cs3org/reva/pull/1925


Changelog for reva 1.10.0 (2021-07-13)
=======================================

The following sections list the changes in reva 1.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1883: Pass directories with trailing slashes to eosclient.GenerateToken
 * Fix #1878: Improve the webdav error handling in the trashbin
 * Fix #1884: Do not send body on failed range request
 * Enh #1744: Add support for lightweight user types

Details
-------

 * Bugfix #1883: Pass directories with trailing slashes to eosclient.GenerateToken

   https://github.com/cs3org/reva/pull/1883

 * Bugfix #1878: Improve the webdav error handling in the trashbin

   The trashbin handles errors better now on the webdav endpoint.

   https://github.com/cs3org/reva/pull/1878

 * Bugfix #1884: Do not send body on failed range request

   Instead of send the error in the body of a 416 response we log it. This prevents the go reverse
   proxy from choking on it and turning it into a 502 Bad Gateway response.

   https://github.com/cs3org/reva/pull/1884

 * Enhancement #1744: Add support for lightweight user types

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

 * Fix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder
 * Fix #1835: Cleanup owncloudsql driver
 * Fix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping
 * Fix #1885: Fix template in eoshomewrapper to use context user rather than resource
 * Fix #1833: Properly handle name collisions for deletes in the owncloud driver
 * Fix #1874: Use the original file mtime during upload
 * Fix #1854: Add the uid/gid to the url for eos
 * Fix #1848: Fill in missing gid/uid number with nobody
 * Fix #1831: Make the ocm-provider endpoint in the ocmd service unprotected
 * Fix #1808: Use empty array in OCS Notifications endpoints
 * Fix #1825: Raise max grpc message size
 * Fix #1828: Send a proper XML header with error messages
 * Chg #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7
 * Enh #1834: Add API key to Mentix GOCDB connector
 * Enh #1855: Minor optimization in parsing EOS ACLs
 * Enh #1873: Update the EOS image tag to be for revad-eos image
 * Enh #1802: Introduce list spaces
 * Enh #1849: Add readonly interceptor
 * Enh #1875: Simplify resource comparison
 * Enh #1827: Support trashbin sub paths in the recycle API

Details
-------

 * Bugfix #1843: Correct Dockerfile path for the reva CLI and alpine3.13 as builder

   This was introduced on https://github.com/cs3org/reva/commit/117adad while porting the
   configuration on .drone.yml to starlark.

   Force golang:alpine3.13 as base image to prevent errors from Make when running on Docker
   <20.10 as it happens on Drone
   ref.https://gitlab.alpinelinux.org/alpine/aports/-/issues/12396

   https://github.com/cs3org/reva/pull/1843
   https://github.com/cs3org/reva/pull/1844
   https://github.com/cs3org/reva/pull/1847

 * Bugfix #1835: Cleanup owncloudsql driver

   Use `owncloudsql` string when returning errors and removed copyMD as it does not need to copy
   metadata from files.

   https://github.com/cs3org/reva/pull/1835

 * Bugfix #1868: Minor fixes to the grpc/http plugin: checksum, url escaping

   https://github.com/cs3org/reva/pull/1868

 * Bugfix #1885: Fix template in eoshomewrapper to use context user rather than resource

   https://github.com/cs3org/reva/pull/1885

 * Bugfix #1833: Properly handle name collisions for deletes in the owncloud driver

   In the owncloud storage driver when we delete a file we append the deletion time to the file name.
   If two fast consecutive deletes happened, the deletion time would be the same and if the two
   files had the same name we ended up with only one file in the trashbin.

   https://github.com/cs3org/reva/pull/1833

 * Bugfix #1874: Use the original file mtime during upload

   The decomposedfs was not using the original file mtime during uploads.

   https://github.com/cs3org/reva/pull/1874

 * Bugfix #1854: Add the uid/gid to the url for eos

   https://github.com/cs3org/reva/pull/1854

 * Bugfix #1848: Fill in missing gid/uid number with nobody

   When an LDAP server does not provide numeric uid or gid properties for a user we now fall back to a
   configurable `nobody` id (default 99).

   https://github.com/cs3org/reva/pull/1848

 * Bugfix #1831: Make the ocm-provider endpoint in the ocmd service unprotected

   https://github.com/cs3org/reva/issues/1751
   https://github.com/cs3org/reva/pull/1831

 * Bugfix #1808: Use empty array in OCS Notifications endpoints

   https://github.com/cs3org/reva/pull/1808

 * Bugfix #1825: Raise max grpc message size

   As a workaround for listing larger folder we raised the `MaxCallRecvMsgSize` to 10MB. This
   should be enough for ~15k files. The proper fix is implementing ListContainerStream in the
   gateway, but we needed a way to test the web ui with larger collections.

   https://github.com/cs3org/reva/pull/1825

 * Bugfix #1828: Send a proper XML header with error messages

   https://github.com/cs3org/reva/pull/1828

 * Change #1828: Remove the oidc provider in order to upgrad mattn/go-sqlite3 to v1.14.7

   In order to upgrade mattn/go-sqlite3 to v1.14.7, the odic provider service is removed, which
   is possible because it is not used anymore

   https://github.com/cs3org/reva/pull/1828
   https://github.com/owncloud/ocis/pull/2209

 * Enhancement #1834: Add API key to Mentix GOCDB connector

   The PI (programmatic interface) of the GOCDB will soon require an API key; this PR adds the
   ability to configure this key in Mentix.

   https://github.com/cs3org/reva/pull/1834

 * Enhancement #1855: Minor optimization in parsing EOS ACLs

   https://github.com/cs3org/reva/pull/1855

 * Enhancement #1873: Update the EOS image tag to be for revad-eos image

   https://github.com/cs3org/reva/pull/1873

 * Enhancement #1802: Introduce list spaces

   The ListStorageSpaces call now allows listing all user homes and shared resources using a
   storage space id. The gateway will forward requests to a specific storage provider when a
   filter by id is given. Otherwise it will query all storage providers. Results will be
   deduplicated. Currently, only the decomposed fs storage driver implements the necessary
   logic to demonstrate the implmentation. A new `/dav/spaces` WebDAV endpoint to directly
   access a storage space is introduced in a separate PR.

   https://github.com/cs3org/reva/pull/1802
   https://github.com/cs3org/reva/pull/1803

 * Enhancement #1849: Add readonly interceptor

   The readonly interceptor could be used to configure a storageprovider in readonly mode. This
   could be handy in some migration scenarios.

   https://github.com/cs3org/reva/pull/1849

 * Enhancement #1875: Simplify resource comparison

   We replaced ResourceEqual with ResourceIDEqual where possible.

   https://github.com/cs3org/reva/pull/1875

 * Enhancement #1827: Support trashbin sub paths in the recycle API

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

 * Fix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests
 * Fix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI
 * Chg #1721: Absolute and relative references
 * Enh #1810: Add arbitrary metadata support to EOS
 * Enh #1774: Add user ID cache warmup to EOS storage driver
 * Enh #1471: EOEGrpc progress. Logging discipline and error handling
 * Enh #1811: Harden public shares signing
 * Enh #1793: Remove the user id from the trashbin key
 * Enh #1795: Increase trashbin restore API compatibility
 * Enh #1516: Use UidNumber and GidNumber fields in User objects
 * Enh #1820: Tag v1.9.0

Details
-------

 * Bugfix #1815: Drone CI - patch the 'store-dev-release' job to fix malformed requests

   Replace the backquotes that were used for the date component of the URL with the
   POSIX-confirmant command substitution '$()'.

   https://github.com/cs3org/reva/pull/1815

 * Bugfix #1765: 'golang:alpine' as base image & CGO_ENABLED just for the CLI

   Some of the dependencies used by revad need CGO to be enabled in order to work. We also need to
   install the 'mime-types' in alpine to correctly detect them on the storage-providers.

   The CGO_ENABLED=0 flag was added to the docker build flags so that it will produce a static
   build. This allows usage of the 'scratch' image for reduction of the docker image size (e.g. the
   reva cli).

   https://github.com/cs3org/reva/issues/1765
   https://github.com/cs3org/reva/pull/1766
   https://github.com/cs3org/reva/pull/1797

 * Change #1721: Absolute and relative references

   We unified the `Reference_Id` end `Reference_Path` types to a combined `Reference` that
   contains both: - a `resource_id` property that can identify a node using a `storage_id` and an
   `opaque_id` - a `path` property that can be used to represent absolute paths as well as paths
   relative to the id based properties. While this is a breaking change it allows passing both:
   absolute as well as relative references.

   https://github.com/cs3org/reva/pull/1721

 * Enhancement #1810: Add arbitrary metadata support to EOS

   https://github.com/cs3org/reva/pull/1810

 * Enhancement #1774: Add user ID cache warmup to EOS storage driver

   https://github.com/cs3org/reva/pull/1774

 * Enhancement #1471: EOEGrpc progress. Logging discipline and error handling

   https://github.com/cs3org/reva/pull/1471

 * Enhancement #1811: Harden public shares signing

   Makes golangci-lint happy as well

   https://github.com/cs3org/reva/pull/1811

 * Enhancement #1793: Remove the user id from the trashbin key

   We don't want to use the users uuid outside of the backend so I removed the id from the trashbin
   file key.

   https://github.com/cs3org/reva/pull/1793

 * Enhancement #1795: Increase trashbin restore API compatibility

   * The precondition were not checked before doing a trashbin restore in the ownCloud dav API.
   Without the checks the API would behave differently compared to the oC10 API. * The restore
   response was missing HTTP headers like `ETag` * Update the name when restoring the file from
   trashbin to a new target name

   https://github.com/cs3org/reva/pull/1795

 * Enhancement #1516: Use UidNumber and GidNumber fields in User objects

   Update instances where CS3API's `User` objects are created and used to use `GidNumber`, and
   `UidNumber` fields instead of storing them in `Opaque` map.

   https://github.com/cs3org/reva/issues/1516

 * Enhancement #1820: Tag v1.9.0

   Bump release number to v1.9.0 as it contains breaking changes related to changing the
   reference type.

   https://github.com/cs3org/reva/pull/1820


Changelog for reva 1.8.0 (2021-06-09)
=======================================

The following sections list the changes in reva 1.8.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1779: Set Content-Type header correctly for ocs requests
 * Fix #1650: Allow fetching shares as the grantee
 * Fix #1693: Fix move in owncloud storage driver
 * Fix #1666: Fix public file shares
 * Fix #1541: Allow for restoring recycle items to different locations
 * Fix #1718: Use the -static ldflag only for the 'build-ci' target
 * Enh #1719: Application passwords CLI
 * Enh #1719: Application passwords management
 * Enh #1725: Create transfer type share
 * Enh #1755: Return file checksum available from the metadata for the EOS driver
 * Enh #1673: Deprecate using errors.New and fmt.Errorf
 * Enh #1723: Open in app workflow using the new API
 * Enh #1655: Improve json marshalling of share protobuf messages
 * Enh #1694: User profile picture capability
 * Enh #1649: Add reliability calculations support to Mentix
 * Enh #1509: Named Service Registration
 * Enh #1643: Cache resources from share getter methods in OCS
 * Enh #1664: Add cache warmup strategy for OCS resource infos
 * Enh #1710: Owncloudsql storage driver
 * Enh #1705: Reduce the size of all the container images built on CI
 * Enh #1669: Mint scope-based access tokens for RBAC
 * Enh #1683: Filter created shares based on type in OCS
 * Enh #1763: Sort share entries alphabetically
 * Enh #1758: Warn user for not recommended go version
 * Enh #1747: Add checksum headers to tus preflight responses
 * Enh #1685: Add share to update response

Details
-------

 * Bugfix #1779: Set Content-Type header correctly for ocs requests

   Before this fix the `Content-Type` header was guessed by `w.Write` because `WriteHeader` was
   called to early. Now the `Content-Type` is set correctly and to the same values as in ownCloud 10

   https://github.com/owncloud/ocis/issues/1779

 * Bugfix #1650: Allow fetching shares as the grantee

   The json backend now allows a grantee to fetch a share by id.

   https://github.com/cs3org/reva/pull/1650

 * Bugfix #1693: Fix move in owncloud storage driver

   When moving a file or folder (includes renaming) the filepath in the cache didn't get updated
   which caused subsequent requests to `getpath` to fail.

   https://github.com/cs3org/reva/issues/1693
   https://github.com/cs3org/reva/issues/1696

 * Bugfix #1666: Fix public file shares

   Fixed stat requests and propfind responses for publicly shared files.

   https://github.com/cs3org/reva/pull/1666

 * Bugfix #1541: Allow for restoring recycle items to different locations

   The CS3 APIs specify a way to restore a recycle item to a different location than the original by
   setting the `restore_path` field in the `RestoreRecycleItemRequest`. This field had not
   been considered until now.

   https://github.com/cs3org/reva/pull/1541
   https://cs3org.github.io/cs3apis/

 * Bugfix #1718: Use the -static ldflag only for the 'build-ci' target

   It is not intended to statically link the generated binaries for local development workflows.
   This resulted on segmentation faults and compiller warnings.

   https://github.com/cs3org/reva/pull/1718

 * Enhancement #1719: Application passwords CLI

   This PR adds the CLI commands `token-list`, `token-create` and `token-remove` to manage
   tokens with limited scope on behalf of registered users.

   https://github.com/cs3org/reva/pull/1719

 * Enhancement #1719: Application passwords management

   This PR adds the functionality to generate authentication tokens with limited scope on behalf
   of registered users. These can be used in third party apps or in case primary user credentials
   cannot be submitted to other parties.

   https://github.com/cs3org/reva/issues/1714
   https://github.com/cs3org/reva/pull/1719
   https://github.com/cs3org/cs3apis/pull/127

 * Enhancement #1725: Create transfer type share

   `transfer-create` creates a share of type transfer.

   https://github.com/cs3org/reva/pull/1725

 * Enhancement #1755: Return file checksum available from the metadata for the EOS driver

   https://github.com/cs3org/reva/pull/1755

 * Enhancement #1673: Deprecate using errors.New and fmt.Errorf

   Previously we were using errors.New and fmt.Errorf to create errors. Now we use the errors
   defined in the errtypes package.

   https://github.com/cs3org/reva/issues/1673

 * Enhancement #1723: Open in app workflow using the new API

   This provides a new `open-in-app` command for the CLI and the implementation on the
   appprovider gateway service for the new API, including the option to specify the appplication
   to use, thus overriding the preconfigured one.

   https://github.com/cs3org/reva/pull/1723

 * Enhancement #1655: Improve json marshalling of share protobuf messages

   Protobuf oneof fields cannot be properly handled by the native json marshaller, and the
   protojson package can only handle proto messages. Previously, we were using a workaround of
   storing these oneof fields separately, which made the code inelegant. Now we marshal these
   messages as strings before marshalling them via the native json package.

   https://github.com/cs3org/reva/pull/1655

 * Enhancement #1694: User profile picture capability

   Based on feedback in the new ownCloud web frontend we want to omit trying to render user avatars
   images / profile pictures based on the backend capabilities. Now the OCS communicates a
   corresponding value.

   https://github.com/cs3org/reva/pull/1694

 * Enhancement #1649: Add reliability calculations support to Mentix

   To make reliability calculations possible, a new exporter has been added to Mentix that reads
   scheduled downtimes from the GOCDB and exposes it through Prometheus metrics.

   https://github.com/cs3org/reva/pull/1649

 * Enhancement #1509: Named Service Registration

   Move away from hardcoding service IP addresses and rely upon name resolution instead. It
   delegates the address lookup to a static in-memory service registry, which can be
   re-implemented in multiple forms.

   https://github.com/cs3org/reva/pull/1509

 * Enhancement #1643: Cache resources from share getter methods in OCS

   In OCS, once we retrieve the shares from the shareprovider service, we stat each of those
   separately to obtain the required info, which introduces a lot of latency. This PR introduces a
   resoource info cache in OCS, which would prevent this latency.

   https://github.com/cs3org/reva/pull/1643

 * Enhancement #1664: Add cache warmup strategy for OCS resource infos

   Recently, a TTL cache was added to OCS to store statted resource infos. This PR adds an interface
   to define warmup strategies and also adds a cbox specific strategy which starts a goroutine to
   initialize the cache with all the valid shares present in the system.

   https://github.com/cs3org/reva/pull/1664

 * Enhancement #1710: Owncloudsql storage driver

   This PR adds a storage driver which connects to a oc10 storage backend (storage + database).
   This allows for running oc10 and ocis with the same backend in parallel.

   https://github.com/cs3org/reva/pull/1710

 * Enhancement #1705: Reduce the size of all the container images built on CI

   Previously, all images were based on golang:1.16 which is built from Debian. Using 'scratch'
   as base, reduces the size of the artifacts well as the attack surface for all the images, plus
   copying the binary from the build step ensures that only the strictly required software is
   present on the final image. For the revad images tagged '-eos', eos-slim is used instead. It is
   still large but it updates the environment as well as the EOS version.

   https://github.com/cs3org/reva/pull/1705

 * Enhancement #1669: Mint scope-based access tokens for RBAC

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

 * Enhancement #1683: Filter created shares based on type in OCS

   https://github.com/cs3org/reva/pull/1683

 * Enhancement #1763: Sort share entries alphabetically

   When showing the list of shares to the end-user, the list was not sorted alphabetically. This PR
   sorts the list of users and groups.

   https://github.com/cs3org/reva/issues/1763

 * Enhancement #1758: Warn user for not recommended go version

   This PR adds a warning while an user is building the source code, if he is using a go version not
   recommended.

   https://github.com/cs3org/reva/issues/1758
   https://github.com/cs3org/reva/pull/1760

 * Enhancement #1747: Add checksum headers to tus preflight responses

   Added `checksum` to the header `Tus-Extension` and added the `Tus-Checksum-Algorithm`
   header.

   https://github.com/owncloud/ocis/issues/1747
   https://github.com/cs3org/reva/pull/1702

 * Enhancement #1685: Add share to update response

   After accepting or rejecting a share the API includes the updated share in the response.

   https://github.com/cs3org/reva/pull/1685
   https://github.com/cs3org/reva/pull/1724


Changelog for reva 1.7.0 (2021-04-19)
=======================================

The following sections list the changes in reva 1.7.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1619: Fixes for enabling file sharing in EOS
 * Fix #1576: Fix etag changing only once a second
 * Fix #1634: Mentix site authorization status changes
 * Fix #1625: Make local file connector more error tolerant
 * Fix #1526: Fix webdav file versions endpoint bugs
 * Fix #1457: Cloning of internal mesh data lost some values
 * Fix #1597: Check for ENOTDIR on readlink error
 * Fix #1636: Skip file check for OCM data transfers
 * Fix #1552: Fix a bunch of trashbin related issues
 * Fix #1: Bump meshdirectory-web to 1.0.2
 * Chg #1562: Modularize api token management in GRAPPA drivers
 * Chg #1452: Separate blobs from metadata in the ocis storage driver
 * Enh #1514: Add grpc test suite for the storage provider
 * Enh #1466: Add integration tests for the s3ng driver
 * Enh #1521: Clarify expected failures
 * Enh #1624: Add wrappers for EOS and EOS Home storage drivers
 * Enh #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType
 * Enh #1411: Make InsecureSkipVerify configurable
 * Enh #1106: Make command to run litmus tests
 * Enh #1502: Bump meshdirectory-web to v1.0.4
 * Enh #1502: New MeshDirectory HTTP service UI frontend with project branding
 * Enh #1405: Quota querying and tree accounting
 * Enh #1527: Add FindAcceptedUsers method to OCM Invite API
 * Enh #1149: Add CLI Commands for OCM invitation workflow
 * Enh #1629: Implement checksums in the owncloud storage
 * Enh #1528: Port drone pipeline definition to starlark
 * Enh #110: Add signature authentication for public links
 * Enh #1495: SQL driver for the publicshare service
 * Enh #1588: Make the additional info attribute for shares configurable
 * Enh #1595: Add site account registration panel
 * Enh #1506: Site Accounts service for API keys
 * Enh #116: Enhance storage registry with virtual views and regular expressions
 * Enh #1513: Add stubs for storage spaces manipulation

Details
-------

 * Bugfix #1619: Fixes for enabling file sharing in EOS

   https://github.com/cs3org/reva/pull/1619

 * Bugfix #1576: Fix etag changing only once a second

   We fixed a problem with the owncloud storage driver only considering the mtime with a second
   resolution for the etag calculation.

   https://github.com/cs3org/reva/pull/1576

 * Bugfix #1634: Mentix site authorization status changes

   If a site changes its authorization status, Mentix did not update its internal data to reflect
   this change. This PR fixes this issue.

   https://github.com/cs3org/reva/pull/1634

 * Bugfix #1625: Make local file connector more error tolerant

   The local file connector caused Reva to throw an exception if the local file for storing site
   data couldn't be loaded. This PR changes this behavior so that only a warning is logged.

   https://github.com/cs3org/reva/pull/1625

 * Bugfix #1526: Fix webdav file versions endpoint bugs

   Etag and error code related bugs have been fixed for the webdav file versions endpoint and
   removed from the expected failures file.

   https://github.com/cs3org/reva/pull/1526

 * Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

 * Bugfix #1597: Check for ENOTDIR on readlink error

   The deconstructed storage driver now handles ENOTDIR errors when `node.Child()` is called
   for a path containing a path segment that is actually a file.

   https://github.com/owncloud/ocis/issues/1239
   https://github.com/cs3org/reva/pull/1597

 * Bugfix #1636: Skip file check for OCM data transfers

   https://github.com/cs3org/reva/pull/1636

 * Bugfix #1552: Fix a bunch of trashbin related issues

   Fixed these issues:

   - Complete: Deletion time in trash bin shows a wrong date - Complete: shared trash status code -
   Partly: invalid webdav responses for unauthorized requests. - Partly: href in trashbin
   PROPFIND response is wrong

   Complete means there are no expected failures left. Partly means there are some scenarios
   left.

   https://github.com/cs3org/reva/pull/1552

 * Bugfix #1: Bump meshdirectory-web to 1.0.2

   Updated meshdirectory-web mod to version 1.0.2 that contains fixes for OCM invite API links
   generation.

   https://github.com/sciencemesh/meshdirectory-web/pull/1

 * Change #1562: Modularize api token management in GRAPPA drivers

   This PR moves the duplicated api token management methods into a seperate utils package

   https://github.com/cs3org/reva/issues/1562

 * Change #1452: Separate blobs from metadata in the ocis storage driver

   We changed the ocis storage driver to keep the file content separate from the metadata by
   storing the blobs in a separate directory. This allows for using a different (potentially
   faster) storage for the metadata.

   **Note** This change makes existing ocis storages incompatible with the new code.

   We also streamlined the ocis and the s3ng drivers so that most of the code is shared between them.

   https://github.com/cs3org/reva/pull/1452

 * Enhancement #1514: Add grpc test suite for the storage provider

   A new test suite has been added which tests the grpc interface to the storage provider. It
   currently runs against the ocis and the owncloud storage drivers.

   https://github.com/cs3org/reva/pull/1514

 * Enhancement #1466: Add integration tests for the s3ng driver

   We extended the integration test suite to also run all tests against the s3ng driver.

   https://github.com/cs3org/reva/pull/1466

 * Enhancement #1521: Clarify expected failures

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

 * Enhancement #1624: Add wrappers for EOS and EOS Home storage drivers

   For CERNBox, we need the mount ID to be configured according to the owner of a resource. Setting
   this in the storageprovider means having different instances of this service to cater to
   different users, which does not scale. This driver forms a wrapper around the EOS driver and
   sets the mount ID according to a configurable mapping based on the owner of the resource.

   https://github.com/cs3org/reva/pull/1624

 * Enhancement #1563: Implement cs3.sharing.collaboration.v1beta1.Share.ShareType

   Interface method Share() in pkg/ocm/share/share.go now has a share type parameter.

   https://github.com/cs3org/reva/pull/1563

 * Enhancement #1411: Make InsecureSkipVerify configurable

   Add `InsecureSkipVerify` field to `metrics.Config` struct and update examples to include
   it.

   https://github.com/cs3org/reva/issues/1411

 * Enhancement #1106: Make command to run litmus tests

   This updates adds an extra make command to run litmus tests via make. `make litmus-test`
   executes the tests.

   https://github.com/cs3org/reva/issues/1106
   https://github.com/cs3org/reva/pull/1543

 * Enhancement #1502: Bump meshdirectory-web to v1.0.4

   Updated meshdirectory-web version to v.1.0.4 bringing multiple UX improvements in provider
   list and map.

   https://github.com/cs3org/reva/issues/1502
   https://github.com/sciencemesh/meshdirectory-web/pull/2
   https://github.com/sciencemesh/meshdirectory-web/pull/3

 * Enhancement #1502: New MeshDirectory HTTP service UI frontend with project branding

   We replaced the temporary version of web frontend of the mesh directory http service with a new
   redesigned & branded one. Because the new version is a more complex Vue SPA that contains image,
   css and other assets, it is now served from a binary package distribution that was generated
   using the [github.com/rakyll/statik](https://github.com/rakyll/statik) package. The
   `http.services.meshdirectory.static` config option was obsoleted by this change.

   https://github.com/cs3org/reva/issues/1502

 * Enhancement #1405: Quota querying and tree accounting

   The ocs api now returns the user quota for the users home storage. Furthermore, the ocis storage
   driver now reads the quota from the extended attributes of the user home or root node and
   implements tree size accounting. Finally, ocdav PROPFINDS now handle the
   `DAV:quota-used-bytes` and `DAV:quote-available-bytes` properties.

   https://github.com/cs3org/reva/pull/1405
   https://github.com/cs3org/reva/pull/1491

 * Enhancement #1527: Add FindAcceptedUsers method to OCM Invite API

   https://github.com/cs3org/reva/pull/1527

 * Enhancement #1149: Add CLI Commands for OCM invitation workflow

   This adds a couple of CLI commands, `ocm-invite-generate` and `ocm-invite-forward` to
   generate and forward ocm invitation tokens respectively.

   https://github.com/cs3org/reva/issues/1149

 * Enhancement #1629: Implement checksums in the owncloud storage

   Implemented checksums in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1629

 * Enhancement #1528: Port drone pipeline definition to starlark

   Having the pipeline definition as a starlark script instead of plain yaml greatly improves the
   flexibility and allows for removing lots of duplicated definitions.

   https://github.com/cs3org/reva/pull/1528

 * Enhancement #110: Add signature authentication for public links

   Implemented signature authentication for public links in addition to the existing password
   authentication. This allows web clients to efficiently download files from password
   protected public shares.

   https://github.com/cs3org/cs3apis/issues/110
   https://github.com/cs3org/reva/pull/1590

 * Enhancement #1495: SQL driver for the publicshare service

   https://github.com/cs3org/reva/pull/1495

 * Enhancement #1588: Make the additional info attribute for shares configurable

   AdditionalInfoAttribute can be configured via the `additional_info_attribute` key in the
   form of a Go template string. If not explicitly set, the default value is `{{.Mail}}`

   https://github.com/cs3org/reva/pull/1588

 * Enhancement #1595: Add site account registration panel

   This PR adds a site account registration panel to the site accounts service. It also removes
   site registration from the xcloud metrics driver.

   https://github.com/cs3org/reva/pull/1595

 * Enhancement #1506: Site Accounts service for API keys

   This update adds a new service to Reva that handles site accounts creation and management.
   Registered sites can be assigned an API key through a simple web interface which is also part of
   this service. This API key can then be used to identify a user and his/her associated (vendor or
   partner) site.

   Furthermore, Mentix was extended to make use of this new service. This way, all sites now have a
   stable and unique site ID that not only avoids ID collisions but also introduces a new layer of
   security (i.e., sites can only be modified or removed using the correct API key).

   https://github.com/cs3org/reva/pull/1506

 * Enhancement #116: Enhance storage registry with virtual views and regular expressions

   Add the functionality to the storage registry service to handle user requests for references
   which can span across multiple storage providers, particularly useful for cases where
   directories are sharded across providers or virtual views are expected.

   https://github.com/cs3org/cs3apis/pull/116
   https://github.com/cs3org/reva/pull/1570

 * Enhancement #1513: Add stubs for storage spaces manipulation

   This PR adds stubs for the storage space CRUD methods in the storageprovider service and makes
   the expired shares janitor configureable in the publicshares SQL driver.

   https://github.com/cs3org/reva/pull/1513


Changelog for reva 1.6.0 (2021-02-16)
=======================================

The following sections list the changes in reva 1.6.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1425: Align href URL encoding with oc10
 * Fix #1461: Fix public link webdav permissions
 * Fix #1457: Cloning of internal mesh data lost some values
 * Fix #1429: Purge non-empty dirs from trash-bin
 * Fix #1408: Get error status from trash-bin response
 * Enh #1451: Render additional share with in ocs sharing api
 * Enh #1424: We categorized the list of expected failures
 * Enh #1434: CERNBox REST driver for groupprovider service
 * Enh #1400: Checksum support
 * Enh #1431: Update npm packages to fix vulnerabilities
 * Enh #1415: Indicate in EOS containers that TUS is not supported
 * Enh #1402: Parse EOS sys ACLs to generate CS3 resource permissions
 * Enh #1477: Set quota when creating home directory in EOS
 * Enh #1416: Use updated etag of home directory even if it is cached
 * Enh #1478: Enhance error handling for grappa REST drivers
 * Enh #1453: Add functionality to share resources with groups
 * Enh #99: Add stubs and manager for groupprovider service
 * Enh #1462: Hash public share passwords
 * Enh #1464: LDAP driver for the groupprovider service
 * Enh #1430: Capture non-deterministic behavior on storages
 * Enh #1456: Fetch user groups in OIDC and LDAP backend
 * Enh #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore
 * Enh #1467: Align default location for xrdcopy binary

Details
-------

 * Bugfix #1425: Align href URL encoding with oc10

   We now use the same percent encoding for URLs in WebDAV href properties as ownCloud 10.

   https://github.com/owncloud/ocis/issues/1120
   https://github.com/owncloud/ocis/issues/1296
   https://github.com/owncloud/ocis/issues/1307
   https://github.com/cs3org/reva/pull/1425
   https://github.com/cs3org/reva/pull/1472

 * Bugfix #1461: Fix public link webdav permissions

   We now correctly render `oc:permissions` on the root collection of a publicly shared folder
   when it has more than read permissions.

   https://github.com/cs3org/reva/pull/1461

 * Bugfix #1457: Cloning of internal mesh data lost some values

   This update fixes a bug in Mentix that caused some (non-critical) values to be lost during data
   cloning that happens internally.

   https://github.com/cs3org/reva/pull/1457

 * Bugfix #1429: Purge non-empty dirs from trash-bin

   This wasn't possible before if the directory was not empty

   https://github.com/cs3org/reva/pull/1429

 * Bugfix #1408: Get error status from trash-bin response

   Previously the status code was gathered from the wrong response.

   https://github.com/cs3org/reva/pull/1408

 * Enhancement #1451: Render additional share with in ocs sharing api

   Recipients can now be distinguished by their email, which is rendered as additional info in the
   ocs api for share and file owners as well as share recipients.

   https://github.com/owncloud/ocis/issues/1190
   https://github.com/cs3org/reva/pull/1451

 * Enhancement #1424: We categorized the list of expected failures

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

 * Enhancement #1434: CERNBox REST driver for groupprovider service

   https://github.com/cs3org/reva/pull/1434

 * Enhancement #1400: Checksum support

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

 * Enhancement #1431: Update npm packages to fix vulnerabilities

   https://github.com/cs3org/reva/pull/1431

 * Enhancement #1415: Indicate in EOS containers that TUS is not supported

   The OCDAV propfind response previously hardcoded the TUS headers due to which clients such as
   phoenix used the TUS protocol for uploads, which EOS doesn't support. Now we pass this property
   as an opaque entry in the containers metadata.

   https://github.com/cs3org/reva/pull/1415

 * Enhancement #1402: Parse EOS sys ACLs to generate CS3 resource permissions

   https://github.com/cs3org/reva/pull/1402

 * Enhancement #1477: Set quota when creating home directory in EOS

   https://github.com/cs3org/reva/pull/1477

 * Enhancement #1416: Use updated etag of home directory even if it is cached

   We cache the home directory and shares folder etags as calculating these is an expensive
   process. But if these directories were updated after the previously calculated etag was
   cached, we can ignore this calculation and directly return the new one.

   https://github.com/cs3org/reva/pull/1416

 * Enhancement #1478: Enhance error handling for grappa REST drivers

   https://github.com/cs3org/reva/pull/1478

 * Enhancement #1453: Add functionality to share resources with groups

   https://github.com/cs3org/reva/pull/1453

 * Enhancement #99: Add stubs and manager for groupprovider service

   Recently, there was a separation of concerns with regard to users and groups in CS3APIs. This PR
   adds the required stubs and drivers for the group manager.

   https://github.com/cs3org/cs3apis/pull/99
   https://github.com/cs3org/cs3apis/pull/102
   https://github.com/cs3org/reva/pull/1358

 * Enhancement #1462: Hash public share passwords

   The share passwords were only base64 encoded. Added hashing using bcrypt with configurable
   hash cost.

   https://github.com/cs3org/reva/pull/1462

 * Enhancement #1464: LDAP driver for the groupprovider service

   https://github.com/cs3org/reva/pull/1464

 * Enhancement #1430: Capture non-deterministic behavior on storages

   As a developer creating/maintaining a storage driver I want to be able to validate the
   atomicity of all my storage driver operations. * Test for: Start 2 uploads, pause the first one,
   let the second one finish first, resume the first one at some point in time. Both uploads should
   finish. Needs to result in 2 versions, last finished is the most recent version. * Test for:
   Start 2 MKCOL requests with the same path, one needs to fail.

   https://github.com/cs3org/reva/pull/1430

 * Enhancement #1456: Fetch user groups in OIDC and LDAP backend

   https://github.com/cs3org/reva/pull/1456

 * Enhancement #1429: Add s3ng storage driver, storing blobs in a s3-compatible blobstore

   We added a new storage driver (s3ng) which stores the file metadata on a local filesystem
   (reusing the decomposed filesystem of the ocis driver) and the actual content as blobs in any
   s3-compatible blobstore.

   https://github.com/cs3org/reva/pull/1429

 * Enhancement #1467: Align default location for xrdcopy binary

   https://github.com/cs3org/reva/pull/1467


Changelog for reva 1.5.1 (2021-01-19)
=======================================

The following sections list the changes in reva 1.5.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1401: Use the user in request for deciding the layout for non-home DAV requests
 * Fix #1413: Re-include the '.git' dir in the Docker images to pass the version tag
 * Fix #1399: Fix ocis trash-bin purge
 * Enh #1397: Bump the Copyright date to 2021
 * Enh #1398: Support site authorization status in Mentix
 * Enh #1393: Allow setting favorites, mtime and a temporary etag
 * Enh #1403: Support remote cloud gathering metrics

Details
-------

 * Bugfix #1401: Use the user in request for deciding the layout for non-home DAV requests

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

 * Bugfix #1413: Re-include the '.git' dir in the Docker images to pass the version tag

   And git SHA to the release tool.

   https://github.com/cs3org/reva/pull/1413

 * Bugfix #1399: Fix ocis trash-bin purge

   Fixes the empty trash-bin functionality for ocis-storage

   https://github.com/owncloud/product/issues/254
   https://github.com/cs3org/reva/pull/1399

 * Enhancement #1397: Bump the Copyright date to 2021

   https://github.com/cs3org/reva/pull/1397

 * Enhancement #1398: Support site authorization status in Mentix

   This enhancement adds support for a site authorization status to Mentix. This way, sites
   registered via a web app can now be excluded until authorized manually by an administrator.

   Furthermore, Mentix now sets the scheme for Prometheus targets. This allows us to also support
   monitoring of sites that do not support the default HTTPS scheme.

   https://github.com/cs3org/reva/pull/1398

 * Enhancement #1393: Allow setting favorites, mtime and a temporary etag

   We now let the ocis driver persist favorites, set temporary etags and the mtime as arbitrary
   metadata.

   https://github.com/owncloud/ocis/issues/567
   https://github.com/cs3org/reva/issues/1394
   https://github.com/cs3org/reva/pull/1393

 * Enhancement #1403: Support remote cloud gathering metrics

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

 * Fix #1385: Run changelog check only if there are changes in the code
 * Fix #1333: Delete sdk unit tests
 * Fix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files
 * Fix #1338: Fix fd leaks
 * Fix #1343: Fix ocis move
 * Fix #551: Fix purging deleted files with the ocis storage
 * Fix #863: Fix dav api for trashbin
 * Fix #204: Fix the ocs share with me response
 * Fix #1351: Fix xattr.Remove error check for macOS
 * Fix #1320: Do not panic on remote.php/dav/files/
 * Fix #1379: Make Jaeger agent usable
 * Fix #1331: Fix capabilities response for multiple client versions
 * Fix #1281: When sharing via ocs look up user by username
 * Fix #1334: Handle removal of public shares by token or ID
 * Chg #990: Replace the user uuid with the username in ocs share responses
 * Enh #1350: Add auth protocol based on user agent
 * Enh #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors
 * Enh #1364: Remove expired Link on Get
 * Enh #1340: Add cache to store UID to UserID mapping in EOS
 * Enh #1154: Add support for the protobuf interface to eos metadata
 * Enh #1154: Merge-rebase from master 10/11/2020
 * Enh #1359: Add cache for calculated etags for home and shares directory
 * Enh #1321: Add support for multiple data transfer protocols
 * Enh #1324: Log expected errors with debug level
 * Enh #1351: Map errtypes to status
 * Enh #1347: Support property to enable health checking on a service
 * Enh #1332: Add import support to Mentix
 * Enh #1371: Use self-hosted Drone CI
 * Enh #1354: Map bad request and unimplement to http status codes
 * Enh #929: Include share types in ocs propfind responses
 * Enh #1328: Add CLI commands for public shares
 * Enh #1388: Support range header in GET requests
 * Enh #1361: Remove expired Link on Access
 * Enh #1386: Docker image for cs3org/revad:VERSION-eos
 * Enh #1368: Calculate and expose actual file permission set

Details
-------

 * Bugfix #1385: Run changelog check only if there are changes in the code

   https://github.com/cs3org/reva/pull/1385

 * Bugfix #1333: Delete sdk unit tests

   These depend on a remote server running reva and thus fail in case of version mismatches.

   https://github.com/cs3org/reva/pull/1333

 * Bugfix #1342: Dav endpoint routing to home storage when request is remote.php/dav/files

   There was a regression in which we were not routing correctly to the right storage depending on
   the url.

   https://github.com/cs3org/reva/pull/1342

 * Bugfix #1338: Fix fd leaks

   There were some left over open file descriptors on simple.go.

   https://github.com/cs3org/reva/pull/1338

 * Bugfix #1343: Fix ocis move

   Use the old node id to build the target path for xattr updates.

   https://github.com/owncloud/ocis/issues/975
   https://github.com/cs3org/reva/pull/1343

 * Bugfix #551: Fix purging deleted files with the ocis storage

   The ocis storage could load the owner information of a deleted file. This caused the storage to
   not be able to purge deleted files.

   https://github.com/owncloud/ocis/issues/551

 * Bugfix #863: Fix dav api for trashbin

   The api was comparing the requested username to the userid.

   https://github.com/owncloud/ocis/issues/863

 * Bugfix #204: Fix the ocs share with me response

   The path of the files shared with me was incorrect.

   https://github.com/owncloud/product/issues/204
   https://github.com/cs3org/reva/pull/1346

 * Bugfix #1351: Fix xattr.Remove error check for macOS

   Previously, we checked the xattr.Remove error only for linux systems. Now macOS is checked
   also

   https://github.com/cs3org/reva/pull/1351

 * Bugfix #1320: Do not panic on remote.php/dav/files/

   Currently requests to /remote.php/dav/files/ result in panics since we cannot longer strip
   the user + destination from the url. This fixes the server response code and adds an error body to
   the response.

   https://github.com/cs3org/reva/pull/1320

 * Bugfix #1379: Make Jaeger agent usable

   Previously, you could not use tracing with jaeger agent because the tracing connector is
   always used instead of the tracing endpoint.

   This PR removes the defaults for collector and tracing endpoint.

   https://github.com/cs3org/reva/pull/1379

 * Bugfix #1331: Fix capabilities response for multiple client versions

   https://github.com/cs3org/reva/pull/1331

 * Bugfix #1281: When sharing via ocs look up user by username

   The ocs api returns usernames when listing share recipients, so the lookup when creating the
   share needs to search the usernames and not the userid.

   https://github.com/cs3org/reva/pull/1281

 * Bugfix #1334: Handle removal of public shares by token or ID

   Previously different drivers handled removing public shares using different means, either
   the token or the ID. Now, both the drivers support both these methods.

   https://github.com/cs3org/reva/pull/1334

 * Change #990: Replace the user uuid with the username in ocs share responses

   The ocs api should not send the users uuid. Replaced the uuid with the username.

   https://github.com/owncloud/ocis/issues/990
   https://github.com/cs3org/reva/pull/1375

 * Enhancement #1350: Add auth protocol based on user agent

   Previously, all available credential challenges are given to the client, for example, basic
   auth, bearer token, etc ... Different clients have different priorities to use one method or
   another, and before it was not possible to force a client to use one method without having a side
   effect on other clients.

   This PR adds the functionality to target a specific auth protocol based on the user agent HTTP
   header.

   https://github.com/cs3org/reva/pull/1350

 * Enhancement #1362: Mark 'store-dev-release' CI step as failed on 4XX/5XX errors

   Prevent the errors while storing new 'daily' releases from going unnoticed on the CI.

   https://github.com/cs3org/reva/pull/1362

 * Enhancement #1364: Remove expired Link on Get

   There is the scenario in which a public link has expired but ListPublicLink has not run,
   accessing a technically expired public share is still possible.

   https://github.com/cs3org/reva/pull/1364

 * Enhancement #1340: Add cache to store UID to UserID mapping in EOS

   Previously, we used to send an RPC to the user provider service for every lookup of user IDs from
   the UID stored in EOS. This PR adds an in-memory lock-protected cache to store this mapping.

   https://github.com/cs3org/reva/pull/1340

 * Enhancement #1154: Add support for the protobuf interface to eos metadata

   https://github.com/cs3org/reva/pull/1154

 * Enhancement #1154: Merge-rebase from master 10/11/2020

   https://github.com/cs3org/reva/pull/1154

 * Enhancement #1359: Add cache for calculated etags for home and shares directory

   Since we store the references in the shares directory instead of actual resources, we need to
   calculate the etag on every list/stat call. This is rather expensive so adding a cache would
   help to a great extent with regard to the performance.

   https://github.com/cs3org/reva/pull/1359

 * Enhancement #1321: Add support for multiple data transfer protocols

   Previously, we had to configure which data transfer protocol to use in the dataprovider
   service. A previous PR added the functionality to redirect requests to different handlers
   based on the request method but that would lead to conflicts if multiple protocols don't
   support mutually exclusive sets of requests. This PR adds the functionality to have multiple
   such handlers simultaneously and the client can choose which protocol to use.

   https://github.com/cs3org/reva/pull/1321
   https://github.com/cs3org/reva/pull/1285/

 * Enhancement #1324: Log expected errors with debug level

   While trying to download a non existing file and reading a non existing attribute are
   technically an error they are to be expected and nothing an admin can or even should act upon.

   https://github.com/cs3org/reva/pull/1324

 * Enhancement #1351: Map errtypes to status

   When mapping errtypes to grpc statuses we now also map bad request and not implemented /
   unsupported cases in the gateway storageprovider.

   https://github.com/cs3org/reva/pull/1351

 * Enhancement #1347: Support property to enable health checking on a service

   This update introduces a new service property called `ENABLE_HEALTH_CHECKS` that must be
   explicitly set to `true` if a service should be checked for its health status. This allows us to
   only enable these checks for partner sites only, skipping vendor sites.

   https://github.com/cs3org/reva/pull/1347

 * Enhancement #1332: Add import support to Mentix

   This update adds import support to Mentix, transforming it into a **Mesh Entity Exchanger**.
   To properly support vendor site management, a new connector that works on a local file has been
   added as well.

   https://github.com/cs3org/reva/pull/1332

 * Enhancement #1371: Use self-hosted Drone CI

   Previously, we used the drone cloud to run the CI for the project. Due to unexpected and sudden
   stop of the service for the cs3org we decided to self-host it.

   https://github.com/cs3org/reva/pull/1371

 * Enhancement #1354: Map bad request and unimplement to http status codes

   We now return a 400 bad request when a grpc call fails with an invalid argument status and a 501 not
   implemented when it fails with an unimplemented status. This prevents 500 errors when a user
   tries to add resources to the Share folder or a storage does not implement an action.

   https://github.com/cs3org/reva/pull/1354

 * Enhancement #929: Include share types in ocs propfind responses

   Added the share types to the ocs propfind response when a resource has been shared.

   https://github.com/owncloud/ocis/issues/929
   https://github.com/cs3org/reva/pull/1329

 * Enhancement #1328: Add CLI commands for public shares

   https://github.com/cs3org/reva/pull/1328

 * Enhancement #1388: Support range header in GET requests

   To allow resuming a download we now support GET requests with a range header.

   https://github.com/owncloud/ocis-reva/issues/12
   https://github.com/cs3org/reva/pull/1388

 * Enhancement #1361: Remove expired Link on Access

   Since there is no background jobs scheduled to wipe out expired resources, for the time being
   public links are going to be removed on a "on demand" basis, meaning whenever there is an API call
   that access the list of shares for a given resource, we will check whether the share is expired
   and delete it if so.

   https://github.com/cs3org/reva/pull/1361

 * Enhancement #1386: Docker image for cs3org/revad:VERSION-eos

   Based on eos:c8_4.8.15 (Centos8, version 4.8.15). To be used when the Reva daemon needs IPC
   with xrootd/eos via stdin/out.

   https://github.com/cs3org/reva/pull/1386
   https://github.com/cs3org/reva/pull/1389

 * Enhancement #1368: Calculate and expose actual file permission set

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

 * Fix #1316: Fix listing shares for nonexisting path
 * Fix #1274: Let the gateway filter invalid references
 * Fix #1269: Handle more eos errors
 * Fix #1297: Check the err and the response status code
 * Fix #1260: Fix file descriptor leak on ocdav put handler
 * Fix #1253: Upload file to storage provider after assembling chunks
 * Fix #1264: Fix etag propagation in ocis driver
 * Fix #1255: Check current node when iterating over path segments
 * Fix #1265: Stop setting propagation xattr on new files
 * Fix #260: Filter share with me requests
 * Fix #1317: Prevent nil pointer when listing shares
 * Fix #1259: Fix propfind response code on forbidden files
 * Fix #1294: Fix error type in read node when file was not found
 * Fix #1258: Update share grants on share update
 * Enh #1257: Add a test user to all sites
 * Enh #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect
 * Enh #1239: Add logic for finding groups to user provider service
 * Enh #1280: Add a Reva SDK
 * Enh #1237: Setup of grpc transfer service and cli
 * Enh #1224: Add SQL driver for share manager
 * Enh #1285: Refactor the uploading files workflow from various clients
 * Enh #1233: Add support for custom CodiMD mimetype

Details
-------

 * Bugfix #1316: Fix listing shares for nonexisting path

   When trying to list shares for a not existing file or folder the ocs sharing implementation no
   longer responds with the wrong status code and broken xml.

   https://github.com/cs3org/reva/pull/1316

 * Bugfix #1274: Let the gateway filter invalid references

   We now filter deleted and unshared entries from the response when listing the shares folder of a
   user.

   https://github.com/cs3org/reva/pull/1274

 * Bugfix #1269: Handle more eos errors

   We now treat E2BIG, EACCES as a permission error, which occur, eg. when acl checks fail and
   return a permission denied error.

   https://github.com/cs3org/reva/pull/1269

 * Bugfix #1297: Check the err and the response status code

   The publicfile handler needs to check the response status code to return proper not pound and
   permission errors in the webdav api.

   https://github.com/cs3org/reva/pull/1297

 * Bugfix #1260: Fix file descriptor leak on ocdav put handler

   File descriptors on the ocdav service, especially on the put handler was leaking http
   connections. This PR addresses this.

   https://github.com/cs3org/reva/pull/1260

 * Bugfix #1253: Upload file to storage provider after assembling chunks

   In the PUT handler for chunked uploads in ocdav, we store the individual chunks in temporary
   file but do not write the assembled file to storage. This PR fixes that.

   https://github.com/cs3org/reva/pull/1253

 * Bugfix #1264: Fix etag propagation in ocis driver

   We now use a new synctime timestamp instead of trying to read the mtime to avoid race conditions
   when the stat request happens too quickly.

   https://github.com/owncloud/product/issues/249
   https://github.com/cs3org/reva/pull/1264

 * Bugfix #1255: Check current node when iterating over path segments

   When checking permissions we were always checking the leaf instead of using the current node
   while iterating over path segments.

   https://github.com/cs3org/reva/pull/1255

 * Bugfix #1265: Stop setting propagation xattr on new files

   We no longer set the propagation flag on a file because it is only evaluated for folders anyway.

   https://github.com/cs3org/reva/pull/1265

 * Bugfix #260: Filter share with me requests

   The OCS API now properly filters share with me requests by path and by share status (pending,
   accepted, rejected, all)

   https://github.com/owncloud/ocis-reva/issues/260
   https://github.com/owncloud/ocis-reva/issues/311
   https://github.com/cs3org/reva/pull/1301

 * Bugfix #1317: Prevent nil pointer when listing shares

   We now handle cases where the grpc connection failed correctly by no longer trying to access the
   response status.

   https://github.com/cs3org/reva/pull/1317

 * Bugfix #1259: Fix propfind response code on forbidden files

   When executing a propfind to a resource owned by another user the service would respond with a
   HTTP 403. In ownCloud 10 the response was HTTP 207. This change sets the response code to HTTP 207
   to stay backwards compatible.

   https://github.com/cs3org/reva/pull/1259

 * Bugfix #1294: Fix error type in read node when file was not found

   The method ReadNode in the ocis storage didn't return the error type NotFound when a file was not
   found.

   https://github.com/cs3org/reva/pull/1294

 * Bugfix #1258: Update share grants on share update

   When a share was updated the share information in the share manager was updated but the grants
   set by the storage provider were not.

   https://github.com/cs3org/reva/pull/1258

 * Enhancement #1257: Add a test user to all sites

   For health monitoring of all mesh sites, we need a special user account that is present on every
   site. This PR adds such a user to each users-*.json file so that every site will have the same test
   user credentials.

   https://github.com/cs3org/reva/pull/1257

 * Enhancement #1234: Resolve a WOPI bridge appProviderURL by extracting its redirect

   Applications served by the WOPI bridge (CodiMD for the time being) require an extra
   redirection as the WOPI bridge itself behaves like a user app. This change returns to the client
   the redirected URL from the WOPI bridge, which is the real application URL.

   https://github.com/cs3org/reva/pull/1234

 * Enhancement #1239: Add logic for finding groups to user provider service

   To create shares with user groups, the functionality for searching for these based on a pattern
   is needed. This PR adds that.

   https://github.com/cs3org/reva/pull/1239

 * Enhancement #1280: Add a Reva SDK

   A Reva SDK has been added to make working with a remote Reva instance much easier by offering a
   high-level API that hides all the underlying details of the CS3API.

   https://github.com/cs3org/reva/pull/1280

 * Enhancement #1237: Setup of grpc transfer service and cli

   The grpc transfer service and cli for it.

   https://github.com/cs3org/reva/pull/1237

 * Enhancement #1224: Add SQL driver for share manager

   This PR adds an SQL driver for the shares manager which expects a schema equivalent to the one
   used in production for CERNBox.

   https://github.com/cs3org/reva/pull/1224

 * Enhancement #1285: Refactor the uploading files workflow from various clients

   Previously, we were implementing the tus client logic in the ocdav service, leading to
   restricting the whole of tus logic to the internal services. This PR refactors that workflow to
   accept incoming requests following the tus protocol while using simpler transmission
   internally.

   https://github.com/cs3org/reva/pull/1285
   https://github.com/cs3org/reva/pull/1314

 * Enhancement #1233: Add support for custom CodiMD mimetype

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

 * Fix #1140: Call the gateway stat method from appprovider
 * Fix #1170: Up and download of file shares
 * Fix #1177: Fix ocis move
 * Fix #1178: Fix litmus failing on ocis storage
 * Fix #237: Fix missing quotes on OCIS-Storage
 * Fix #1210: No longer swallow permissions errors in the gateway
 * Fix #1183: Handle eos EPERM as permission denied
 * Fix #1206: No longer swallow permissions errors
 * Fix #1207: No longer swallow permissions errors in ocdav
 * Fix #1161: Cache display names in ocs service
 * Fix #1216: Add error handling for invalid references
 * Enh #1205: Allow using the username when accessing the users home
 * Enh #1131: Use updated cato to display nested package config in parent docs
 * Enh #1213: Check permissions in ocis driver
 * Enh #1202: Check permissions in owncloud driver
 * Enh #1228: Add GRPC stubs for CreateSymlink method
 * Enh #1174: Add logic in EOS FS for maintaining same inode across file versions
 * Enh #1142: Functionality to map home directory to different storage providers
 * Enh #1190: Add Blackbox Exporter support to Mentix
 * Enh #1229: New gateway datatx service
 * Enh #1225: Allow setting the owner when using the ocis driver
 * Enh #1180: Introduce ocis driver treetime accounting
 * Enh #1208: Calculate etags on-the-fly for shares directory and home folder

Details
-------

 * Bugfix #1140: Call the gateway stat method from appprovider

   The appprovider service used to directly pass the stat request to the storage provider
   bypassing the gateway, which resulted in errors while handling share children as they are
   resolved in the gateway path.

   https://github.com/cs3org/reva/pull/1140

 * Bugfix #1170: Up and download of file shares

   The shared folder logic in the gateway storageprovider was not allowing file up and downloads
   for single file shares. We now check if the reference is actually a file to determine if up /
   download should be allowed.

   https://github.com/cs3org/reva/pull/1170

 * Bugfix #1177: Fix ocis move

   When renaming a file we updating the name attribute on the wrong node, causing the path
   construction to use the wrong name. This fixes the litmus move_coll test.

   https://github.com/cs3org/reva/pull/1177

 * Bugfix #1178: Fix litmus failing on ocis storage

   We now ignore the `no data available` error when removing a non existing metadata attribute,
   which is ok because we are trying to delete it anyway.

   https://github.com/cs3org/reva/issues/1178
   https://github.com/cs3org/reva/pull/1179

 * Bugfix #237: Fix missing quotes on OCIS-Storage

   Etags have to be enclosed in quotes ". Return correct etags on OCIS-Storage.

   https://github.com/owncloud/product/issues/237
   https://github.com/cs3org/reva/pull/1232

 * Bugfix #1210: No longer swallow permissions errors in the gateway

   The gateway is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `status.NewPermissionDenied` or `status.NewInternal` instead of reusing the original
   response status.

   https://github.com/cs3org/reva/pull/1210

 * Bugfix #1183: Handle eos EPERM as permission denied

   We now treat EPERM errors, which occur, eg. when acl checks fail and return a permission denied
   error.

   https://github.com/cs3org/reva/pull/1183

 * Bugfix #1206: No longer swallow permissions errors

   The storageprovider is no longer ignoring permissions errors. It will now report them
   properly using `status.NewPermissionDenied(...)` instead of `status.NewInternal(...)`

   https://github.com/cs3org/reva/pull/1206

 * Bugfix #1207: No longer swallow permissions errors in ocdav

   The ocdav api is no longer ignoring permissions errors. It will now check the status for
   `rpc.Code_CODE_PERMISSION_DENIED` codes and report them properly using
   `http.StatusForbidden` instead of `http.StatusInternalServerError`

   https://github.com/cs3org/reva/pull/1207

 * Bugfix #1161: Cache display names in ocs service

   The ocs list shares endpoint may need to fetch the displayname for multiple different users. We
   are now caching the lookup fo 60 seconds to save redundant RPCs to the users service.

   https://github.com/cs3org/reva/pull/1161

 * Bugfix #1216: Add error handling for invalid references

   https://github.com/cs3org/reva/pull/1216
   https://github.com/cs3org/reva/pull/1218

 * Enhancement #1205: Allow using the username when accessing the users home

   We now allow using the userid and the username when accessing the users home on the `/dev/files`
   endpoint.

   https://github.com/cs3org/reva/pull/1205

 * Enhancement #1131: Use updated cato to display nested package config in parent docs

   Previously, in case of nested packages, we just had a link pointing to the child package. Now we
   copy the nested package's documentation to the parent itself to make it easier for devs.

   https://github.com/cs3org/reva/pull/1131

 * Enhancement #1213: Check permissions in ocis driver

   We are now checking grant permissions in the ocis storage driver.

   https://github.com/cs3org/reva/pull/1213

 * Enhancement #1202: Check permissions in owncloud driver

   We are now checking grant permissions in the owncloud storage driver.

   https://github.com/cs3org/reva/pull/1202

 * Enhancement #1228: Add GRPC stubs for CreateSymlink method

   https://github.com/cs3org/reva/pull/1228

 * Enhancement #1174: Add logic in EOS FS for maintaining same inode across file versions

   This PR adds the functionality to maintain the same inode across various versions of a file by
   returning the inode of the version folder which remains constant. It requires extra metadata
   operations so a flag is provided to disable it.

   https://github.com/cs3org/reva/pull/1174

 * Enhancement #1142: Functionality to map home directory to different storage providers

   We hardcode the home path for all users to /home. This forbids redirecting requests for
   different users to multiple storage providers. This PR provides the option to map the home
   directories of different users using user attributes.

   https://github.com/cs3org/reva/pull/1142

 * Enhancement #1190: Add Blackbox Exporter support to Mentix

   This update extends Mentix to export a Prometheus SD file specific to the Blackbox Exporter
   which will be used for initial health monitoring. Usually, Prometheus requires its targets to
   only consist of the target's hostname; the BBE though expects a full URL here. This makes
   exporting two distinct files necessary.

   https://github.com/cs3org/reva/pull/1190

 * Enhancement #1229: New gateway datatx service

   Represents the CS3 datatx module in the gateway.

   https://github.com/cs3org/reva/pull/1229

 * Enhancement #1225: Allow setting the owner when using the ocis driver

   To support the metadata storage we allow setting the owner of the root node so that subsequent
   requests with that owner can be used to manage the storage.

   https://github.com/cs3org/reva/pull/1225

 * Enhancement #1180: Introduce ocis driver treetime accounting

   We added tree time accounting to the ocis storage driver which is modeled after [eos synctime
   accounting](http://eos-docs.web.cern.ch/eos-docs/configuration/namespace.html#enable-subtree-accounting).
   It can be enabled using the new `treetime_accounting` option, which defaults to `false` The
   `tmtime` is stored in an extended attribute `user.ocis.tmtime`. The treetime accounting is
   enabled for nodes which have the `user.ocis.propagation` extended attribute set to `"1"`.
   Currently, propagation is in sync.

   https://github.com/cs3org/reva/pull/1180

 * Enhancement #1208: Calculate etags on-the-fly for shares directory and home folder

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

 * Fix #1124: Do not swallow 'not found' errors in Stat
 * Enh #1125: Rewire dav files to the home storage
 * Enh #559: Introduce ocis storage driver
 * Enh #1118: Metrics module can be configured to retrieve metrics data from file

Details
-------

 * Bugfix #1124: Do not swallow 'not found' errors in Stat

   Webdav needs to determine if a file exists to return 204 or 201 response codes. When stating a non
   existing resource the NOT_FOUND code was replaced with an INTERNAL error code. This PR passes
   on a NOT_FOUND status code in the gateway.

   https://github.com/cs3org/reva/pull/1124

 * Enhancement #1125: Rewire dav files to the home storage

   If the user specified in the dav files URL matches the current one, rewire it to use the
   webDavHandler which is wired to the home storage.

   This fixes path mapping issues.

   https://github.com/cs3org/reva/pull/1125

 * Enhancement #559: Introduce ocis storage driver

   We introduced a now storage driver `ocis` that deconstructs a filesystem and uses a node first
   approach to implement an efficient lookup of files by path as well as by file id.

   https://github.com/cs3org/reva/pull/559

 * Enhancement #1118: Metrics module can be configured to retrieve metrics data from file

   - Export site metrics in Prometheus #698

   https://github.com/cs3org/reva/pull/1118


Changelog for reva 1.2.0 (2020-08-25)
=======================================

The following sections list the changes in reva 1.2.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1099: Do not restore recycle entry on purge
 * Fix #1091: Allow listing the trashbin
 * Fix #1103: Restore and delete trash items via ocs
 * Fix #1090: Ensure ignoring public stray shares
 * Fix #1064: Ensure ignoring stray shares
 * Fix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient
 * Fix #1115: Owncloud driver - propagate mtime on RemoveGrant
 * Fix #1111: Handle redirection prefixes when extracting destination from URL
 * Enh #1101: Add UID and GID in ldap auth driver
 * Enh #1077: Add calens check to verify changelog entries in CI
 * Enh #1072: Refactor Reva CLI with prompts
 * Enh #1079: Get file info using fxids from EOS
 * Enh #1088: Update LDAP user driver
 * Enh #1114: System information metrics cleanup
 * Enh #1071: System information included in Prometheus metrics
 * Enh #1094: Add logic for resolving storage references over webdav

Details
-------

 * Bugfix #1099: Do not restore recycle entry on purge

   This PR fixes a bug in the EOSFS driver that was restoring a deleted entry when asking for its
   permanent purge. EOS does not have the functionality to purge individual files, but the whole
   recycle of the user.

   https://github.com/cs3org/reva/pull/1099

 * Bugfix #1091: Allow listing the trashbin

   The trashbin endpoint expects the userid, not the username.

   https://github.com/cs3org/reva/pull/1091

 * Bugfix #1103: Restore and delete trash items via ocs

   The OCS api was not passing the correct key and references to the CS3 API. Furthermore, the
   owncloud storage driver was constructing the wrong target path when restoring.

   https://github.com/cs3org/reva/pull/1103

 * Bugfix #1090: Ensure ignoring public stray shares

   When using the json public shares manager, it can be the case we found a share with a resource_id
   that no longer exists.

   https://github.com/cs3org/reva/pull/1090

 * Bugfix #1064: Ensure ignoring stray shares

   When using the json shares manager, it can be the case we found a share with a resource_id that no
   longer exists. This PR aims to address his case by changing the contract of getPath and return
   the result of the STAT call instead of a generic error, so we can check the cause.

   https://github.com/cs3org/reva/pull/1064

 * Bugfix #1082: Minor fixes in reva cmd, gateway uploads and smtpclient

   https://github.com/cs3org/reva/pull/1082
   https://github.com/cs3org/reva/pull/1116

 * Bugfix #1115: Owncloud driver - propagate mtime on RemoveGrant

   When removing a grant the mtime change also needs to be propagated. Only affectsn the owncluod
   storage driver.

   https://github.com/cs3org/reva/pull/1115

 * Bugfix #1111: Handle redirection prefixes when extracting destination from URL

   The move function handler in ocdav extracts the destination path from the URL by removing the
   base URL prefix from the URL path. This would fail in case there is a redirection prefix. This PR
   takes care of that and it also allows zero-size uploads for localfs.

   https://github.com/cs3org/reva/pull/1111

 * Enhancement #1101: Add UID and GID in ldap auth driver

   The PR https://github.com/cs3org/reva/pull/1088/ added the functionality to lookup UID
   and GID from the ldap user provider. This PR adds the same to the ldap auth manager.

   https://github.com/cs3org/reva/pull/1101

 * Enhancement #1077: Add calens check to verify changelog entries in CI

   https://github.com/cs3org/reva/pull/1077

 * Enhancement #1072: Refactor Reva CLI with prompts

   The current CLI is a bit cumbersome to use with users having to type commands containing all the
   associated config with no prompts or auto-completes. This PR refactors the CLI with these
   functionalities.

   https://github.com/cs3org/reva/pull/1072

 * Enhancement #1079: Get file info using fxids from EOS

   This PR supports getting file information from EOS using the fxid value.

   https://github.com/cs3org/reva/pull/1079

 * Enhancement #1088: Update LDAP user driver

   The LDAP user driver can now fetch users by a single claim / attribute. Use an `attributefilter`
   like `(&(objectclass=posixAccount)({{attr}}={{value}}))` in the driver section.

   It also adds the uid and gid to the users opaque properties so that eos can use them for chown and
   acl operations.

   https://github.com/cs3org/reva/pull/1088

 * Enhancement #1114: System information metrics cleanup

   The system information metrics are now based on OpenCensus instead of the Prometheus client
   library. Furthermore, its initialization was moved out of the Prometheus HTTP service to keep
   things clean.

   https://github.com/cs3org/reva/pull/1114

 * Enhancement #1071: System information included in Prometheus metrics

   System information is now included in the main Prometheus metrics exposed at `/metrics`.

   https://github.com/cs3org/reva/pull/1071

 * Enhancement #1094: Add logic for resolving storage references over webdav

   This PR adds the functionality to resolve webdav references using the ocs webdav service by
   passing the resource's owner's token. This would need to be changed in production.

   https://github.com/cs3org/reva/pull/1094


Changelog for reva 1.1.0 (2020-08-11)
=======================================

The following sections list the changes in reva 1.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1069: Pass build time variables while compiling
 * Fix #1047: Fix missing idp check in GetUser of demo userprovider
 * Fix #1038: Do not stat shared resources when downloading
 * Fix #1034: Fixed some error reporting strings and corresponding logs
 * Fix #1046: Fixed resolution of fileid in GetPathByID
 * Fix #1052: Ocfs: Lookup user to render template properly
 * Fix #1024: Take care of trailing slashes in OCM package
 * Fix #1025: Use lower-case name for changelog directory
 * Fix #1042: List public shares only created by the current user
 * Fix #1051: Disallow sharing the shares directory
 * Enh #1035: Refactor AppProvider workflow
 * Enh #1059: Improve timestamp precision while logging
 * Enh #1037: System information HTTP service
 * Enh #995: Add UID and GID to the user object from user package

Details
-------

 * Bugfix #1069: Pass build time variables while compiling

   We provide the option of viewing various configuration and version options in both reva CLI as
   well as the reva daemon, but we didn't actually have these values in the first place. This PR adds
   that info at compile time.

   https://github.com/cs3org/reva/pull/1069

 * Bugfix #1047: Fix missing idp check in GetUser of demo userprovider

   We've added a check for matching idp in the GetUser function of the demo userprovider

   https://github.com/cs3org/reva/issues/1047

 * Bugfix #1038: Do not stat shared resources when downloading

   Previously, we statted the resources in all download requests resulting in failures when
   downloading references. This PR fixes that by statting only in case the resource is not present
   in the shares folder. It also fixes a bug where we allowed uploading to the mount path, resulting
   in overwriting the user home directory.

   https://github.com/cs3org/reva/pull/1038

 * Bugfix #1034: Fixed some error reporting strings and corresponding logs

   https://github.com/cs3org/reva/pull/1034

 * Bugfix #1046: Fixed resolution of fileid in GetPathByID

   Following refactoring of fileid generations in the local storage provider, this ensures
   fileid to path resolution works again.

   https://github.com/cs3org/reva/pull/1046

 * Bugfix #1052: Ocfs: Lookup user to render template properly

   Currently, the username is used to construct paths, which breaks when mounting the `owncloud`
   storage driver at `/oc` and then expecting paths that use the username like
   `/oc/einstein/foo` to work, because they will mismatch the path that is used from propagation
   which uses `/oc/u-u-i-d` as the root, giving an `internal path outside root` error

   https://github.com/cs3org/reva/pull/1052

 * Bugfix #1024: Take care of trailing slashes in OCM package

   Previously, we assumed that the OCM endpoints would have trailing slashes, failing in case
   they didn't. This PR fixes that.

   https://github.com/cs3org/reva/pull/1024

 * Bugfix #1025: Use lower-case name for changelog directory

   When preparing a new release, the changelog entries need to be copied to the changelog folder
   under docs. In a previous change, all these folders were made to have lower case names,
   resulting in creation of a separate folder.

   https://github.com/cs3org/reva/pull/1025

 * Bugfix #1042: List public shares only created by the current user

   When running ocis, the public links created by a user are visible to all the users under the
   'Shared with others' tab. This PR fixes that by returning only those links which are created by a
   user themselves.

   https://github.com/cs3org/reva/pull/1042

 * Bugfix #1051: Disallow sharing the shares directory

   Previously, it was possible to create public links for and share the shares directory itself.
   However, when the recipient tried to accept the share, it failed. This PR prevents the creation
   of such shares in the first place.

   https://github.com/cs3org/reva/pull/1051

 * Enhancement #1035: Refactor AppProvider workflow

   Simplified the app-provider configuration: storageID is worked out automatically and UIURL
   is suppressed for now. Implemented the new gRPC protocol from the gateway to the appprovider.

   https://github.com/cs3org/reva/pull/1035

 * Enhancement #1059: Improve timestamp precision while logging

   Previously, the timestamp associated with a log just had the hour and minute, which made
   debugging quite difficult. This PR increases the precision of the associated timestamp.

   https://github.com/cs3org/reva/pull/1059

 * Enhancement #1037: System information HTTP service

   This service exposes system information via an HTTP endpoint. This currently only includes
   Reva version information but can be extended easily. The information are exposed in the form of
   Prometheus metrics so that we can gather these in a streamlined way.

   https://github.com/cs3org/reva/pull/1037

 * Enhancement #995: Add UID and GID to the user object from user package

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

 * Fix #941: Fix initialization of json share manager
 * Fix #1006: Check if SMTP credentials are nil
 * Chg #965: Remove protocol from partner domains to match gocdb config
 * Enh #986: Added signing key capability
 * Enh #922: Add tutorial for deploying WOPI and Reva locally
 * Enh #979: Skip changelog enforcement for bot PRs
 * Enh #965: Enforce adding changelog in make and CI
 * Enh #1016: Do not enforce changelog on release
 * Enh #969: Allow requests to hosts with unverified certificates
 * Enh #914: Make httpclient configurable
 * Enh #972: Added a site locations exporter to Mentix
 * Enh #1000: Forward share invites to the provider selected in meshdirectory
 * Enh #1002: Pass the link to the meshdirectory service in token mail
 * Enh #1008: Use proper logging for ldap auth requests
 * Enh #970: Add required headers to SMTP client to prevent being tagged as spam
 * Enh #996: Split LDAP user filters
 * Enh #1007: Update go-tus version
 * Enh #1004: Update github.com/go-ldap/ldap to v3
 * Enh #974: Add functionality to create webdav references for OCM shares

Details
-------

 * Bugfix #941: Fix initialization of json share manager

   When an empty shares.json file existed the json share manager would fail while trying to
   unmarshal the empty file.

   https://github.com/cs3org/reva/issues/941
   https://github.com/cs3org/reva/pull/940

 * Bugfix #1006: Check if SMTP credentials are nil

   Check if SMTP credentials are nil before passing them to the SMTPClient, causing it to crash.

   https://github.com/cs3org/reva/pull/1006

 * Change #965: Remove protocol from partner domains to match gocdb config

   Minor changes for OCM cross-partner testing.

   https://github.com/cs3org/reva/pull/965

 * Enhancement #986: Added signing key capability

   The ocs capabilities can now hold the boolean flag to indicate url signing endpoint and
   middleware are available

   https://github.com/cs3org/reva/pull/986

 * Enhancement #922: Add tutorial for deploying WOPI and Reva locally

   Add a new tutorial on how to run Reva and Wopiserver together locally

   https://github.com/cs3org/reva/pull/922

 * Enhancement #979: Skip changelog enforcement for bot PRs

   Skip changelog enforcement for bot PRs.

   https://github.com/cs3org/reva/pull/979

 * Enhancement #965: Enforce adding changelog in make and CI

   When adding a feature or fixing a bug, a changelog needs to be specified, failing which the build
   wouldn't pass.

   https://github.com/cs3org/reva/pull/965

 * Enhancement #1016: Do not enforce changelog on release

   While releasing a new version of Reva, make release was failing because it was enforcing a
   changelog entry.

   https://github.com/cs3org/reva/pull/1016

 * Enhancement #969: Allow requests to hosts with unverified certificates

   Allow OCM to send requests to other mesh providers with the option of skipping certificate
   verification.

   https://github.com/cs3org/reva/pull/969

 * Enhancement #914: Make httpclient configurable

   - Introduce Options for the httpclient (#914)

   https://github.com/cs3org/reva/pull/914

 * Enhancement #972: Added a site locations exporter to Mentix

   Mentix now offers an endpoint that exposes location information of all sites in the mesh. This
   can be used in Grafana's world map view to show the exact location of every site.

   https://github.com/cs3org/reva/pull/972

 * Enhancement #1000: Forward share invites to the provider selected in meshdirectory

   Added a share invite forward OCM endpoint to the provider links (generated when a user picks a
   target provider in the meshdirectory service web interface), together with an invitation
   token and originating provider domain passed to the service via query params.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1000

 * Enhancement #1002: Pass the link to the meshdirectory service in token mail

   Currently, we just forward the token and the original user's domain when forwarding an OCM
   invite token and expect the user to frame the forward invite URL. This PR instead passes the link
   to the meshdirectory service, from where the user can pick the provider they want to accept the
   invite with.

   https://github.com/sciencemesh/sciencemesh/issues/139
   https://github.com/cs3org/reva/pull/1002

 * Enhancement #1008: Use proper logging for ldap auth requests

   Instead of logging to stdout we now log using debug level logging or error level logging in case
   the configured system user cannot bind to LDAP.

   https://github.com/cs3org/reva/pull/1008

 * Enhancement #970: Add required headers to SMTP client to prevent being tagged as spam

   Mails being sent through the client, specially through unauthenticated SMTP were being
   tagged as spam due to missing headers.

   https://github.com/cs3org/reva/pull/970

 * Enhancement #996: Split LDAP user filters

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

 * Enhancement #1007: Update go-tus version

   The lib now uses go mod which should help golang to sort out dependencies when running `go mod
   tidy`.

   https://github.com/cs3org/reva/pull/1007

 * Enhancement #1004: Update github.com/go-ldap/ldap to v3

   In the current version of the ldap lib attribute comparisons are case sensitive. With v3
   `GetEqualFoldAttributeValue` is introduced, which allows a case insensitive comparison.
   Which AFAICT is what the spec says: see
   https://github.com/go-ldap/ldap/issues/129#issuecomment-333744641

   https://github.com/cs3org/reva/pull/1004

 * Enhancement #974: Add functionality to create webdav references for OCM shares

   Webdav references will now be created in users' shares directory with the target set to the
   original resource's location in their mesh provider.

   https://github.com/cs3org/reva/pull/974


Changelog for reva 0.1.0 (2020-03-18)
=======================================

The following sections list the changes in reva 0.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Enh #402: Build daily releases
 * Enh #416: Improve developer experience
 * Enh #468: remove vendor support
 * Enh #545: simplify configuration
 * Enh #561: improve the documentation
 * Enh #562: support home storages

Details
-------

 * Enhancement #402: Build daily releases

   Reva was not building releases of commits to the master branch. Thanks to @zazola.

   Commit-based released are generated every time a PR is merged into master. These releases are
   available at: https://reva-releases.web.cern.ch

   https://github.com/cs3org/reva/pull/402

 * Enhancement #416: Improve developer experience

   Reva provided the option to be run with a single configuration file by using the -c config flag.

   This PR adds the flag -dev-dir than can point to a directory containing multiple config files.
   The reva daemon will launch a new process per configuration file.

   Kudos to @refs.

   https://github.com/cs3org/reva/pull/416

 * Enhancement #468: remove vendor support

   Because @dependabot cannot update in a clean way the vendor dependecies Reva removed support
   for vendored dependencies inside the project.

   Dependencies will continue to be versioned but they will be downloaded when compiling the
   artefacts.

   https://github.com/cs3org/reva/pull/468
   https://github.com/cs3org/reva/pull/524

 * Enhancement #545: simplify configuration

   Reva configuration was difficul as many of the configuration parameters were not providing
   sane defaults. This PR and the related listed below simplify the configuration.

   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/536
   https://github.com/cs3org/reva/pull/568

 * Enhancement #561: improve the documentation

   Documentation has been improved and can be consulted here: https://reva.link

   https://github.com/cs3org/reva/pull/561
   https://github.com/cs3org/reva/pull/545
   https://github.com/cs3org/reva/pull/568

 * Enhancement #562: support home storages

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

 * Enh #334: Create release procedure for Reva

Details
-------

 * Enhancement #334: Create release procedure for Reva

   Reva did not have any procedure to release versions. This PR brings a new tool to release Reva
   versions (tools/release) and prepares the necessary files for artefact distributed made
   from Drone into Github pages.

   https://github.com/cs3org/reva/pull/334


