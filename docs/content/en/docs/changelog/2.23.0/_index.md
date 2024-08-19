
---
title: "v2.23.0"
linkTitle: "v2.23.0"
weight: 40
description: >
  Changelog for Reva v2.23.0 (2024-08-19)
---

Changelog for reva 2.23.0 (2024-08-19)
=======================================

The following sections list the changes in reva 2.23.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4802: Block overwriting mountpoints
*   Fix #4782: Fixed the response code when copying from a share to a personal space
*   Fix #4805: Fix creating spaces
*   Fix #4651: Fix deleting space shares
*   Fix #4808: Fixed bugs in the owncloudsql storage driver
*   Enh #4772: Allow configuring grpc max connection age
*   Enh #4784: Bump tusd to v2
*   Enh #4478: Hellofs
*   Enh #4744: Respect service transport
*   Enh #4812: Concurrent stat requests when listing shares
*   Enh #4798: Update go-ldap to v3.4.8

Details
-------

*   Bugfix #4802: Block overwriting mountpoints

   This blocks overwriting mountpoints through the webdav COPY api. It is now returning a bad
   request when attempting to overwrite a mountpoint.

   https://github.com/cs3org/reva/pull/4802
   https://github.com/cs3org/reva/pull/4796
   https://github.com/cs3org/reva/pull/4786
   https://github.com/cs3org/reva/pull/4785

*   Bugfix #4782: Fixed the response code when copying from a share to a personal space

   We fixed the response code when copying a file from a share to a personal space with a secure view
   role.

   https://github.com/owncloud/ocis/issues/9482
   https://github.com/cs3org/reva/pull/4782

*   Bugfix #4805: Fix creating spaces

   We fixed a problem where it wasn't possible to create new spaces when running on a non-writable
   working directory.

   https://github.com/cs3org/reva/pull/4805

*   Bugfix #4651: Fix deleting space shares

   We no longer check if a share is an ocm sharee if listng ocm shares has been disabled anyway. This
   allows unsharing space shares.

   https://github.com/cs3org/reva/pull/4651

*   Bugfix #4808: Fixed bugs in the owncloudsql storage driver

   https://github.com/cs3org/reva/pull/4808

*   Enhancement #4772: Allow configuring grpc max connection age

   We added a GRPC_MAX_CONNECTION_AGE env var that allows limiting the lifespan of connections.
   A closed connection triggers grpc clients to do a new DNS lookup to pick up new IPs.

   https://github.com/cs3org/reva/pull/4772

*   Enhancement #4784: Bump tusd to v2

   Bump tusd pkg to v2.4.0

   https://github.com/cs3org/reva/pull/4784

*   Enhancement #4478: Hellofs

   We added a minimal hello world filesystem as an example for a read only storage driver.

   https://github.com/cs3org/reva/pull/4478

*   Enhancement #4744: Respect service transport

   The service registry now takes into account the service transport when creating grpc clients.
   This allows using `dns`, `unix` and `kubernetes` as the protocol in addition to `tcp`. `dns`
   will turn the gRPC client into a [Thick
   Client](https://grpc.io/blog/grpc-load-balancing/#thick-client) that can look up
   multiple endpoints via DNS. `kubernetes` will use
   [github.com/sercand/kuberesolver](https://github.com/sercand/kuberesolver) to
   connect to the kubernetes API and pick up service changes. Furthermore, we enabled round robin
   load balancing for the [default transparent retry configuration of
   gRPC](https://grpc.io/docs/guides/retry/#retry-configuration).

   https://github.com/cs3org/reva/pull/4744

*   Enhancement #4812: Concurrent stat requests when listing shares

   The sharesstorageprovider now concurrently stats the accepted shares when listing the share
   jail. The default number of 5 workers can be changed by setting the `max_concurrency` value in
   the config map.

   https://github.com/cs3org/reva/pull/4812

*   Enhancement #4798: Update go-ldap to v3.4.8

   Update go-ldap/ldap/v3 to the latest upstream release to include the latest bugfixes and
   enhancements.

   https://github.com/cs3org/reva/pull/4798

