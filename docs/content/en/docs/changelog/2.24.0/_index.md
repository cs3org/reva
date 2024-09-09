
---
title: "v2.24.0"
linkTitle: "v2.24.0"
weight: 40
description: >
  Changelog for Reva v2.24.0 (2024-09-09)
---

Changelog for reva 2.24.0 (2024-09-09)
=======================================

The following sections list the changes in reva 2.24.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4820: Fix response code when upload a file over locked
*   Fix #4837: Fix OCM userid encoding
*   Fix #4823: Return etag for ocm shares
*   Fix #4822: Allow listing directory trash items by key
*   Enh #4816: Ignore resharing requests
*   Enh #4817: Added a new role space editor without versions
*   Enh #4829: Added a new roles viewer/editor with ListGrants
*   Enh #4828: New event: UserSignedIn
*   Enh #4836: Publish an event when an OCM invite is generated

Details
-------

*   Bugfix #4820: Fix response code when upload a file over locked

   We fixed a bug where the response code was incorrect when uploading a file over a locked file.

   https://github.com/owncloud/ocis/issues/7638
   https://github.com/cs3org/reva/pull/4820

*   Bugfix #4837: Fix OCM userid encoding

   We now base64 encode the remote userid and provider as the local federated user id. This allows
   us to always differentiate them from local users and unpack the encoded user id and provider
   when making requests to the remote ocm provider.

   https://github.com/owncloud/ocis/issues/9927
   https://github.com/cs3org/reva/pull/4837
   https://github.com/cs3org/reva/pull/4833

*   Bugfix #4823: Return etag for ocm shares

   The ocm remote storage now passes on the etag returned in the PROPFIND response.

   https://github.com/owncloud/ocis/issues/9534
   https://github.com/cs3org/reva/pull/4823

*   Bugfix #4822: Allow listing directory trash items by key

   The storageprovider now passes on the key without inventing a `/` as the relative path when it
   was not present at the end of the key. This allows differentiating requests that want to get the
   trash item of a folder itself (where the relative path is empty) or listing the children of a
   folder in the trash (where the relative path at least starts with a `/`).

   We also fixed the `/dav/spaces` endpoint to not invent a `/` at the end of URLs to allow clients to
   actually make these different requests.

   As a byproduct we now return the size of trashed items.

   https://github.com/cs3org/reva/pull/4822
   https://github.com/cs3org/reva/pull/4818

*   Enhancement #4816: Ignore resharing requests

   We now ignore resharing permissions. Instead of returning BadRequest we just reduce the
   permissions.

   https://github.com/cs3org/reva/pull/4816

*   Enhancement #4817: Added a new role space editor without versions

   We add a new role space editor without list and restore version permissions.

   https://github.com/owncloud/ocis/issues/9699
   https://github.com/cs3org/reva/pull/4817

*   Enhancement #4829: Added a new roles viewer/editor with ListGrants

   We add a new roles space viewer/editor with ListGrants permissions.

   https://github.com/owncloud/ocis/issues/9701
   https://github.com/cs3org/reva/pull/4829

*   Enhancement #4828: New event: UserSignedIn

   There is a new Event that cam be triggered when a user signs in

   https://github.com/cs3org/reva/pull/4828

*   Enhancement #4836: Publish an event when an OCM invite is generated

   The ocm generate-invite endpoint now publishes an event whenever an invitation is requested
   and generated. This event can be subscribed to by other services to react to the generated
   invitation.

   https://github.com/owncloud/ocis/issues/9583
   https://github.com/cs3org/reva/pull/4836
   https://github.com/cs3org/reva/pull/4832

