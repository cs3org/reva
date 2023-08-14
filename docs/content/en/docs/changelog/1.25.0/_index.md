
---
title: "v1.25.0"
linkTitle: "v1.25.0"
weight: 40
description: >
  Changelog for Reva v1.25.0 (2023-08-14)
---

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


