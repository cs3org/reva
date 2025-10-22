
---
title: "v3.1.0"
linkTitle: "v3.1.0"
weight: 999690
description: >
  Changelog for Reva v3.1.0 (2025-08-26)
---

Changelog for reva 3.1.0 (2025-08-26)
=======================================

The following sections list the changes in reva 3.1.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #5218: Check EOS errorcode on CreateHome
 * Fix #5236: Approvider failed to create files
 * Fix #5264: Make COPY requests work again
 * Fix #5237: Mountpoints response
 * Fix #5232: Public link downloadURL
 * Fix #5252: Remove fileid from link virtual folder
 * Fix #5251: Share roles
 * Fix #5270: Filter available share roles based on file type
 * Fix #5242: Paths in shared by me
 * Fix #5265: Add version directory to propfind response
 * Fix #5261: Sharing with guest accounts in spaces now works
 * Fix #5277: Bug in GetStorageSpace
 * Fix #5244: Wopi: fixed access token TTL
 * Fix #5266: Remove deprecated dependency go-render
 * Fix #5255: Make public links work in spaces
 * Enh #5276: Clean up unused dependencies
 * Enh #5274: Use stime instead of mtime for directories
 * Enh #5185: Refactor docs and tutorials
 * Enh #5253: Use TLS for EOS gRPC connections
 * Enh #5275: Extend project db schema
 * Enh #5225: Add create permission for drives (storage space)
 * Enh #5233: Add trace ID to responses
 * Enh #5215: Add private link to propfind response
 * Enh #5272: Added web URL support for shares
 * Enh #5280: Make COPY for folders also work on spaces
 * Enh #5222: Cephfs - inode to path reverse resolution
 * Enh #5235: Allow multi-user share in OCS
 * Enh #5278: Add support for new EOS project quota nodes
 * Enh #5279: Remove dead code
 * Enh #5260: Support for updating space
 * Enh #5254: Allow from and to for trashbin in headers

Details
-------

 * Bugfix #5218: Check EOS errorcode on CreateHome

   When checking whether a user already has a home, we did not specifically check whether the error
   returned was NOT_FOUND, leading us to try to create a home when it may already exist (which
   creates new backup jobs, etc.). Now, we only run CreateHome when EOS reports NOT_FOUND

   https://github.com/cs3org/reva/pull/5218

 * Bugfix #5236: Approvider failed to create files

   In https://github.com/cs3org/reva/pull/4864, a new header was introduced for sending
   content lengths to the datagateway. This header was missing in the appprovider, causing it to
   fail when creating new files.

   https://github.com/cs3org/reva/pull/5236

 * Bugfix #5264: Make COPY requests work again

   COPY requests were broken, because during the upload-part of the copy, no Content-Length
   header was set.

   https://github.com/cs3org/reva/pull/5264

 * Bugfix #5237: Mountpoints response

   Ensures web shows the shares (shared with me) properly and is able to navigate into them.

   https://github.com/cs3org/reva/pull/5237

 * Bugfix #5232: Public link downloadURL

   Removes `/public` prefix from resource path when building the download URL.

   https://github.com/cs3org/reva/pull/5232

 * Bugfix #5252: Remove fileid from link virtual folder

   The front-end expects to not have a file ID on the root of a public link when it is a virtual folder
   around a single file share, for it to automatically open in the default app. The file id of this
   virtual folder has now been removed.

   Additionally, this also fixes the `OC-Checksum: Invalid:` header on downloads of public link
   shared files

   https://github.com/cs3org/reva/pull/5252

 * Bugfix #5251: Share roles

   - share type returned as 'edit' - new id for uploader role (before: same as manager) - role labels
   replaced by unifiedrole id - added file-editor and spaces-related roles to definitions

   https://github.com/cs3org/reva/pull/5251

 * Bugfix #5270: Filter available share roles based on file type

   https://github.com/cs3org/reva/pull/5270

 * Bugfix #5242: Paths in shared by me

   Paths in "Shared by me" were broken: the first path of the path was missing. This is now fixed.

   https://github.com/cs3org/reva/pull/5242

 * Bugfix #5265: Add version directory to propfind response

   PROPFINDs to <resource>/v return a list of all versions of a resource. This list should start
   with a reference to the version directory itself, which in turn is filtered out by the
   front-end. This entry was missing after a refactor of the versions to make them
   spaces-compatible. This change now fixes this.

   https://github.com/cs3org/reva/pull/5265

 * Bugfix #5261: Sharing with guest accounts in spaces now works

   We now fetch the users from the GW and use this info to create the share, instead of passing this
   info directly. Additionally, we don't set `recursive` when setting an attribute on a file.

   https://github.com/cs3org/reva/pull/5261

 * Bugfix #5277: Bug in GetStorageSpace

   https://github.com/cs3org/reva/pull/5277

 * Bugfix #5244: Wopi: fixed access token TTL

   https://github.com/cs3org/reva/pull/5244

 * Bugfix #5266: Remove deprecated dependency go-render

   The package `go-render` is deprecated and no longer available on Github. It has therefore been
   removed from Reva.

   https://github.com/cs3org/reva/pull/5266

 * Bugfix #5255: Make public links work in spaces

   Opening public links in spaces is currently broken. This is fixed by: * not needing a space ID for
   public links * supporting a GET directly on a public link

   https://github.com/cs3org/reva/pull/5255

 * Enhancement #5276: Clean up unused dependencies

   https://github.com/cs3org/reva/pull/5276

 * Enhancement #5274: Use stime instead of mtime for directories

   https://github.com/cs3org/reva/pull/5274

 * Enhancement #5185: Refactor docs and tutorials

   With this PR we move all example configurations out to a dedicated repository,
   https://github.com/cs3org/reva-configs, and the tutorials to the Wiki. We also remove all
   obsoleted content, and keep the auto-generated doc on a dedicated folder.

   This is to help the community further develop the documentation and configuration on separate
   repositories.

   https://github.com/cs3org/reva/pull/5185

 * Enhancement #5253: Use TLS for EOS gRPC connections

   By default, we now use TLS for EOS gRPC connections. Falling back to non-TLS connections is only
   allowed when allow_insecure is set to true.

   https://github.com/cs3org/reva/pull/5253

 * Enhancement #5275: Extend project db schema

   We added a number of fields useful to 2nd and 3rd level support to the database schema of projects

   https://github.com/cs3org/reva/pull/5275

 * Enhancement #5225: Add create permission for drives (storage space)

   Used by the WebUI to present the "Create space" options.

   https://github.com/cs3org/reva/pull/5225

 * Enhancement #5233: Add trace ID to responses

   - REVA trace ID added to responses to help with debugging and tracing requests.

   https://github.com/cs3org/reva/pull/5233

 * Enhancement #5215: Add private link to propfind response

   - new `privatelink` property in the PROPFIND response - `privatelink` is NOT a "permanent
   link", as it's a path based link to the resource

   https://github.com/cs3org/reva/pull/5215

 * Enhancement #5272: Added web URL support for shares

   https://github.com/cs3org/reva/pull/5272

 * Enhancement #5280: Make COPY for folders also work on spaces

   https://github.com/cs3org/reva/pull/5280

 * Enhancement #5222: Cephfs - inode to path reverse resolution

   This enhancement introduces a way to do inode to path reverse resolution. This implementation
   first queries the ceph monitor to find the active ceph MDS (metadata server), and then queries
   the MDS to find the path from an inode using the dump inode command.

   https://github.com/cs3org/reva/pull/5222

 * Enhancement #5235: Allow multi-user share in OCS

   Sending multiple POST requests for multiple users leads to parallel calls to EOS, which
   suffers from a critical race condition when setting ACLs. So, now the reva OCS endpoint
   supports sending multiple comma-seperated users.

   https://github.com/cs3org/reva/pull/5235

 * Enhancement #5278: Add support for new EOS project quota nodes

   For EOS projects, quota nodes used to be set under the service account of the project on the path
   /eos/project

   This has been changed to using GID=99 and having the path of the project be the quota node

   This change introduces support for the new system

   https://github.com/cs3org/reva/pull/5278

 * Enhancement #5279: Remove dead code

   Removed the unused SpacesHandler and related methods from the DAV layer

   https://github.com/cs3org/reva/pull/5279

 * Enhancement #5260: Support for updating space

   This PR adds support for updating spaces to libregraph, specifically the description and
   thumbnail of a space. Additionally, the projects catalogue now directly implements the
   methods of the spaces registry.

   https://github.com/cs3org/reva/pull/5260

 * Enhancement #5254: Allow from and to for trashbin in headers

   Currently, from and to values for trashbin listing are passed as query parameters. With the new
   DAV library on the frontend, it is easier to send these as headers. Reva now accepts both, with
   query parameters having priority.

   https://github.com/cs3org/reva/pull/5254


