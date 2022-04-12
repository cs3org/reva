
---
title: "v2.2.0"
linkTitle: "v2.2.0"
weight: 40
description: >
  Changelog for Reva v2.2.0 (2022-04-12)
---

Changelog for reva 2.2.0 (2022-04-12)
=======================================

The following sections list the changes in reva 2.2.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #3373: Fix the permissions attribute in propfind responses
 * Fix #2721: Fix locking and public link scope checker to make the WOPI server work
 * Fix #2668: Minor cleanup
 * Fix #2692: Ensure that the host in the ocs config endpoint has no protocol
 * Fix #2709: Decomposed FS: return precondition failed if already locked
 * Chg #2687: Allow link with no or edit permission
 * Chg #2658: Small clean up of the ocdav code
 * Enh #2691: Decomposed FS: return a reference to the parent
 * Enh #2708: Rework LDAP configuration of user and group providers
 * Enh #2665: Add embeddable ocdav go micro service
 * Enh #2715: Introduced quicklinks
 * Enh #3370: Enable all spaces members to list public shares
 * Enh #3370: Enable space members to list shares inside the space
 * Enh #2717: Add definitions for user and group events

Details
-------

 * Bugfix #3373: Fix the permissions attribute in propfind responses

   Fixed the permissions that are returned when doing a propfind on a project space.

   https://github.com/owncloud/ocis/issues/3373
   https://github.com/cs3org/reva/pull/2713

 * Bugfix #2721: Fix locking and public link scope checker to make the WOPI server work

   We've fixed the locking implementation to use the CS3api instead of the temporary opaque
   values. We've fixed the scope checker on public links to allow the OpenInApp actions.

   These fixes have been done to use the cs3org/wopiserver with REVA edge.

   https://github.com/cs3org/reva/pull/2721

 * Bugfix #2668: Minor cleanup

   - The `chunk_folder` config option is unused - Prevent a panic when looking up spaces

   https://github.com/cs3org/reva/pull/2668

 * Bugfix #2692: Ensure that the host in the ocs config endpoint has no protocol

   We've fixed the host info in the ocs config endpoint so that it has no protocol, as ownCloud 10
   doesn't have it.

   https://github.com/cs3org/reva/pull/2692
   https://github.com/owncloud/ocis/pull/3113

 * Bugfix #2709: Decomposed FS: return precondition failed if already locked

   We've fixed the return code from permission denied to precondition failed if a user tries to
   lock an already locked file.

   https://github.com/cs3org/reva/pull/2709

 * Change #2687: Allow link with no or edit permission

   Allow the creation of links with no permissions. These can be used to navigate to a file that a
   user has access to. Allow setting edit permission on single file links (create and delete are
   still blocked) Introduce endpoint to get information about a given token

   https://github.com/cs3org/reva/pull/2687

 * Change #2658: Small clean up of the ocdav code

   Cleaned up the ocdav code to make it more readable and in one case a bit faster.

   https://github.com/cs3org/reva/pull/2658

 * Enhancement #2691: Decomposed FS: return a reference to the parent

   We've implemented the changes from cs3org/cs3apis#167 in the DecomposedFS, so that a stat on a
   resource always includes a reference to the parent of the resource.

   https://github.com/cs3org/reva/pull/2691

 * Enhancement #2708: Rework LDAP configuration of user and group providers

   We reworked to LDAP configuration of the LDAP user and group provider to share a common
   configuration scheme. Additionally the LDAP configuration no longer relies on templating
   LDAP filters in the configuration which is error prone and can be confusing. Additionally the
   providers are now somewhat more flexible about the group membership schema. Instead of only
   supporting RFC2307 (posixGroup) style groups. It's now possible to also use standard LDAP
   groups (groupOfName/groupOfUniqueNames) which track group membership by DN instead of
   username (the behaviour is switched automatically depending on the group_objectclass
   setting).

   The new LDAP configuration basically looks this:

   ```ini [grpc.services.userprovider.drivers.ldap] uri="ldaps://localhost:636"
   insecure=true user_base_dn="ou=testusers,dc=owncloud,dc=com"
   group_base_dn="ou=testgroups,dc=owncloud,dc=com" user_filter=""
   user_objectclass="posixAccount" group_filter="" group_objectclass="posixGroup"
   bind_username="cn=admin,dc=owncloud,dc=com" bind_password="admin"
   idp="http://localhost:20080"

   [grpc.services.userprovider.drivers.ldap.user_schema] id="entryuuid"
   displayName="displayName" userName="cn"

   [grpc.services.userprovider.drivers.ldap.group_schema] id="entryuuid"
   displayName="cn" groupName="cn" member="memberUID" ```

   `uri` defines the LDAP URI of the destination Server

   `insecure` allows to disable TLS Certifictate Validation (for development setups)

   `user_base_dn`/`group_base_dn` define the search bases for users and groups

   `user_filter`/`group_filter` allow to define additional LDAP filter of users and groups.
   This could be e.g. `(objectclass=owncloud)` to match for an additional objectclass.

   `user_objectclass`/`group_objectclass` define the main objectclass of Users and Groups.
   These are used to construct the LDAP filters

   `bind_username`/`bind_password` contain the authentication information for the LDAP
   connections

   The `user_schema` and `group_schema` sections define the mapping from CS3 user/group
   attributes to LDAP Attributes

   https://github.com/cs3org/reva/issues/2122
   https://github.com/cs3org/reva/issues/2124
   https://github.com/cs3org/reva/pull/2708

 * Enhancement #2665: Add embeddable ocdav go micro service

   The new `pkg/micro/ocdav` package implements a go micro compatible version of the ocdav
   service.

   https://github.com/cs3org/reva/pull/2665

 * Enhancement #2715: Introduced quicklinks

   We now support Quicklinks. When creating a link with flag "quicklink=true", no new link will be
   created when a link already exists.

   https://github.com/cs3org/reva/pull/2715

 * Enhancement #3370: Enable all spaces members to list public shares

   Enhanced the json and cs3 public share manager so that it lists shares in spaces for all members.

   https://github.com/owncloud/ocis/issues/3370
   https://github.com/cs3org/reva/pull/2697

 * Enhancement #3370: Enable space members to list shares inside the space

   If there are shared resources in a space then all members are allowed to see those shares. The
   json share manager was enhanced to check if the user is allowed to see a share by checking the
   grants on a resource.

   https://github.com/owncloud/ocis/issues/3370
   https://github.com/cs3org/reva/pull/2674
   https://github.com/cs3org/reva/pull/2710

 * Enhancement #2717: Add definitions for user and group events

   Enhance the events package with definitions for user and group events.

   https://github.com/cs3org/reva/pull/2717
   https://github.com/cs3org/reva/pull/2724


