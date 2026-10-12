
---
title: "v3.13.0"
linkTitle: "v3.13.0"
weight: 999570
description: >
  Changelog for Reva v3.13.0 (2026-10-06)
---

Changelog for reva 3.13.0 (2026-10-06)
=======================================

The following sections list the changes in reva 3.13.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Sec #5839: Enforce transport TLS floor and scheme policy
 * Sec #5836: Guard inbound share discovery against SSRF
 * Sec #5838: Guard remaining OCM discovery inputs
 * Sec #5837: Guard receiver OCM and WebDAV connections
 * Sec #5834: Bound peer-controlled OCM JSON
 * Fix #5825: Init admin connection instead of mounting
 * Fix #5858: Set ceph user thread groups per thread
 * Fix #5840: A grpc or http block with no services is no longer rejected
 * Fix #5861: The lightweight scope for updating ocm shares
 * Fix #5804: Rjobs timeouts + registry issues
 * Fix #5816: Incorrect name for labelsprovider
 * Fix #5791: Fixes for the EOSC Node
 * Fix #5811: Report folder size in public links
 * Fix #5817: Reconnect the service registry when publishing fails
 * Fix #5849: Seperate the eos user and userid cache
 * Fix #5813: Make service names unique per transport
 * Enh #5826: Add embedded processing failure mode
 * Enh #5692: Add the Reva Admin API
 * Enh #5815: Discover app providers through the service registry
 * Enh #5644: Implement LoginFlow (apptokens)
 * Enh #5843: Create a home for lightweight accounts on login
 * Enh #5814: Allow LW accounts to use modern paths
 * Enh #5860: Log the resource id on the access line
 * Enh #5859: Log who accessed a resource and who owns it
 * Enh #5768: Small cleanup of notification system
 * Enh #5833: Add shared OCM HTTP transport package
 * Enh #5741: Add periodic job to check orphaned shares
 * Enh #5771: Replace backup_job_id with backup_name in projects table
 * Enh #5841: Fail over to another node when a peer cannot be reached
 * Enh #5665: Add a service registry for service discovery
 * Enh #5746: Implement shallow reconcile job
 * Enh #5780: Add a takeout service to export a user's home directory
 * Enh #5801: Cache for app passwords used by sync client

Details
-------

 * Security #5839: Enforce transport TLS floor and scheme policy

   The shared OCM HTTP transport now requires TLS 1.2 or newer on every client (including trusted
   and insecure modes), rejects plain HTTP on public-only clients except an explicit
   literal-loopback opt-in, and rejects HTTPS-to-HTTP redirects on public-only HTTP clients.
   The address classifier also closes bounded gaps for the IANA special-purpose ranges
   (0.0.0.0/8, 198.18.0.0/15, 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24,
   240.0.0.0/4, 64:ff9b:1::/48, 2001:db8::/32), keeping the PCP and TURN anycast exceptions
   (RFC 7723/8155) and the well-known NAT64 unwrap.

   Explicit federation CIDR ranges do not relax that scheme policy, the TLS minimum, or redirect
   enforcement. A configured private address is still HTTPS-only, and a redirect is checked
   against the same address policy as the dial.

   https://github.com/cs3org/reva/pull/5839

 * Security #5836: Guard inbound share discovery against SSRF

   Unauthenticated inbound `/shares` discovery now uses a public-only HTTP client built once at
   handler init and reused per request, closing the SSRF where a sender-derived server could
   steer the inbound service at private networks. Loopback is allowed only behind an explicit
   `allow_loopback_federation` flag for the local two-provider integration topology; both
   local integration fixtures declare the exception. The discovery timeout is now configurable
   via `ocm_client_timeout` (seconds, default 10, preserving the existing inbound-discovery
   timeout). Operators can opt the inbound public-only client into honoring `HTTP_PROXY`,
   `HTTPS_PROXY`, and `NO_PROXY` via `ocm_client_use_env_proxy` (off by default; the
   public-only client stays direct unless explicitly enabled).

   Controlled federation deployments with private RFC 1918 / IPv6 ULA destinations can opt the
   inbound public-only discovery client into a narrow address exception via
   `allowed_federation_cidrs` (off by default; each entry must be a canonical CIDR wholly
   inside `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, or `fc00::/7`). An invalid
   element fails service startup before any discovery runs: no partially accepted list is
   installed and the router is not served. Ordinary public destinations remain permitted,
   loopback stays gated behind `allow_loopback_federation`, and unrelated private ranges
   remain denied.

   https://github.com/cs3org/reva/pull/5836

 * Security #5838: Guard remaining OCM discovery inputs

   The open provider authorizer's GetInfoByDomain and the ScienceMesh directory-listed
   provider discovery now use the shared public-only OCM client. The operator-configured
   directory fetch stays on the trusted client; only provider URLs taken from the unsigned
   directory response (and request-supplied WAYF discovery) go through the public-only
   client. Loopback is disabled for open-authorizer discovery. Both untrusted paths bypass
   environment proxies. Blocked targets are rejected before any request reaches the server.

   Operators may now configure an explicit `allowed_federation_cidrs` exception list for the
   public-only OCM discovery clients (ScienceMesh WAYF and the open provider authorizer). The
   list is empty by default and admits only canonical RFC 1918 IPv4 or IPv6 ULA prefixes; invalid
   entries abort initialization before any directory fetch. Directory fetches stay trusted;
   listed providers and request-supplied discovery remain public-only with the configured
   exception only. TLS verification, loopback, and proxy controls stay independent.

   https://github.com/cs3org/reva/pull/5838

 * Security #5837: Guard receiver OCM and WebDAV connections

   Inbound legacy WebDAV type probes, received-share OCM discovery and token exchange, and
   received-share WebDAV operations now all use the public-only transport from the shared OCM
   client package. One stored OCM client guards both discovery and token exchange; one driver
   helper builds every gowebdav client and always applies the guarded transport, including the
   retry and one-off upload paths. Loopback is allowed only behind an explicit
   `allow_loopback_federation` flag on the received-storage config; private targets stay
   denied. Enforcement is at dial time, not in URI syntax validation. Operators can opt the
   received public-only clients into honoring `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`
   via `ocm_use_env_proxy` on the received-storage config (off by default; the public-only
   clients stay direct unless explicitly enabled).

   Both received transports, the OCM discovery and token-exchange client and the WebDAV round
   tripper, share the explicit private ranges from allowed_federation_cidrs on the
   received-storage config.

   https://github.com/cs3org/reva/pull/5837

 * Security #5834: Bound peer-controlled OCM JSON

   Every OCM control-plane response body and peer-controlled error snippet is now capped at a 1
   MiB default limit through `readOCMBody` and `decodeOCMJSON` helpers, which read the body
   once and return `ErrResponseTooLarge` for oversized input. This prevents
   decode-after-drain regressions in `NewShare` and `InviteAccepted` and bounds the existing
   unsigned directory JSON at the same limit.

   https://github.com/cs3org/reva/pull/5834

 * Bugfix #5825: Init admin connection instead of mounting

   The ceph admin connection was mounted instead of initialized, which worked when the key had
   mounting permissions but it does not anymore

   https://github.com/cs3org/reva/pull/5825

 * Bugfix #5858: Set ceph user thread groups per thread

   Syscall.Setgroups sets the groups for ALL threads of the process, which caused race
   conditions when multiple users were using the same process. Similarly, when a user thread
   finished, the original groups were restored on ALL threads.

   This is fixed by using unix.Setgroups, which only applies to the calling thread.

   https://github.com/cs3org/reva/pull/5858

 * Bugfix #5840: A grpc or http block with no services is no longer rejected

   Declaring a block that carries only settings, such as a [grpc] with nothing but
   control_address, failed at startup with "grpc.services must be a map". The services key was
   read with a type assertion, which also fails when the key is simply absent. A process whose only
   grpc listener is its control channel can now start.

   https://github.com/cs3org/reva/pull/5840

 * Bugfix #5861: The lightweight scope for updating ocm shares

   The lightweight scope for updating ocm shares was missing and lightweight users were unable to
   process embedded shares.

   https://github.com/cs3org/reva/pull/5861

 * Bugfix #5804: Rjobs timeouts + registry issues

   * Jobs that are killed because of an overlap are no longer seen as a failure and are not retried *
   Daemons that cannot reach the gateway die * Failed resolutions are retried for a few times * The
   scheduler reads only its own schedule keys instead of listing the whole bucket, which timed out
   and stopped all periodic jobs

   https://github.com/cs3org/reva/pull/5804

 * Bugfix #5816: Incorrect name for labelsprovider

   The labels provider was incorrectly named labels and this caused an issue where the other
   services could not find it via the service registry

   https://github.com/cs3org/reva/pull/5816

 * Bugfix #5791: Fixes for the EOSC Node

   https://github.com/cs3org/reva/pull/5791

 * Bugfix #5811: Report folder size in public links

   https://github.com/cs3org/reva/pull/5811

 * Bugfix #5817: Reconnect the service registry when publishing fails

   A process whose NATS write failed marked its registry connection as lost and queued the write,
   but only the watch path ever reconnected. A process with a healthy watcher therefore never
   reconnected: it kept queueing its own heartbeats, silently, while its peers aged its nodes out
   and stopped being able to resolve it, until the process was restarted. Writes now reconnect
   too, so a NATS outage recovers on the next heartbeat, and a reconnect no longer leaves the
   previous connection open.

   https://github.com/cs3org/reva/pull/5817

 * Bugfix #5849: Seperate the eos user and userid cache

   Fixes a bug where an invalid value was stored due to the shared cache, essentially the routine
   that updates missing values updated an incorrect key with a UserId instead of a User.

   https://github.com/cs3org/reva/pull/5849

 * Bugfix #5813: Make service names unique per transport

   Currently, multiple services with the same name (e.g. `preferences`), but using different
   transports (`http` vs `grpc`) could conflict. This caused connections to be randomly to
   either of the services. We now bind this to specific transports, so a service is queried with a
   transport.

   https://github.com/cs3org/reva/pull/5813

 * Enhancement #5826: Add embedded processing failure mode

   Embedded transfers may fail due to a number of reasons, an internal service could be down or the
   remote repository could be unreachable. Instead of being stuck in the "Transferring" state,
   shares are now put in the "Rejected" state so that this can be surfaced to the user in a reasonable
   way.

   https://github.com/cs3org/reva/pull/5826

 * Enhancement #5692: Add the Reva Admin API

   Reva gains an operator API for inspecting and safely operating a running deployment, driven by
   a new `reva admin` command. It is authenticated, audited, and off by default — it turns on only
   where a process is configured with an admin group.

   At its core is a lightweight control channel that every reva process runs next to its normal
   services. Through it the admin can reach a live service instance to read its state or ask it to
   perform an operation (an "invocation"). Targets are resolved through the service registry,
   so a single command can address one instance, every instance of a service, or the whole fleet,
   and the results come back merged.

   With this an operator can see what the fleet is doing — list services and their health, read and
   diff configuration, read or follow logs, trace a single request or user across services, dump
   goroutines, and watch an instance's in-flight request activity — and act on it — set a
   process's log level at runtime, take instances out of and back into rotation for maintenance,
   drive the background jobs runner (list and enqueue jobs, trigger or cancel runs), and
   impersonate a user.

   On the same host it needs no login: it is reached over a Unix socket and authorised by the caller's
   OS identity. Remotely, an operator first steps up to a short-lived, admin-only token.

   https://github.com/cs3org/reva/pull/5692

 * Enhancement #5815: Discover app providers through the service registry

   The app registry kept its own copy of the running app providers, built from a registration each
   app provider pushed once, a few seconds after starting. That copy was keyed by address, so app
   providers sharing one address collapsed into a single entry and all but one became unreachable
   by name, and it was lost whenever the registry restarted, since nothing ever pushed again.

   App providers are ordinary reva services, so they are already in the service registry with a
   reachable address and a liveness state. They now describe the app they serve in their node
   metadata, and the app registry reads them from there. An app is identified by its app name rather
   than by an address, it stops being offered when its providers go away, it comes back on its own
   when they return, and an app served by several nodes is load balanced across them.

   As a consequence the app registry no longer has drivers: there is one implementation,
   configured directly under `[grpc.services.appregistry]`, and the `static` driver
   together with its `[grpc.services.appregistry.drivers.static]` section is gone.
   `AddAppProvider` now returns `UNIMPLEMENTED`, and the `app_provider_url` setting of the
   app provider has been removed.

   https://github.com/cs3org/reva/pull/5815

 * Enhancement #5644: Implement LoginFlow (apptokens)

   Add support for a new login flow, so that different sync clients can: - be registered and removed
   individually - use a token per sync client, instead of relying on basic auth

   This uses NextCloud's /login/v2 flow.

   https://github.com/cs3org/reva/pull/5644

 * Enhancement #5843: Create a home for lightweight accounts on login

   When `lightweight_home_layout` is set in the gateway, a lightweight account that cannot
   reach the folder at that path on login triggers `CreateHome` on the storage provider holding
   it. The EOS driver then runs `create_lightweight_home_hook`, which creates the folder and
   shares it with the account.

   https://github.com/cs3org/reva/pull/5843/

 * Enhancement #5814: Allow LW accounts to use modern paths

   Allows LW accounts to use the more modern path version without `/remote.php/`

   https://github.com/cs3org/reva/pull/5814

 * Enhancement #5860: Log the resource id on the access line

   The owner alone says somebody else's data was reached, not which data. The resource id
   distinguishes them, and unlike the share id it is already in hand.

   https://github.com/cs3org/reva/pull/5860

 * Enhancement #5859: Log who accessed a resource and who owns it

   The HTTP log middleware runs outside authentication and sees neither value, so ocdav records
   them on the context logger, which already carries the trace id.

   https://github.com/cs3org/reva/pull/5859

 * Enhancement #5768: Small cleanup of notification system

   https://github.com/cs3org/reva/pull/5768

 * Enhancement #5833: Add shared OCM HTTP transport package

   Introduces `pkg/ocm/client` with `TransportConfig` and trusted/public-only HTTP client
   constructors, centralizing the transport and address-policy logic previously inlined in
   `ocmd`. The public-only dial guard refuses non-public targets and rejects IPv6 addresses
   carrying a zone suffix, which would otherwise bypass the NAT64 and denied-prefix checks.
   Public-only clients default to direct mode and ignore environment proxies; a service can opt
   in to honoring `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` via the new `UseEnvProxy` transport
   field. Trusted clients honor environment proxies regardless. Existing `ocmd.OCMClient`
   callers are unchanged.

   Public-only clients accept an explicit private-network exception list. The list is empty by
   default, so private addresses stay denied until a caller installs prefixes parsed by
   `ParseFederationCIDRs`. Trusted clients do not consult that list and keep their existing
   proxy behavior.

   https://github.com/cs3org/reva/pull/5833

 * Enhancement #5741: Add periodic job to check orphaned shares

   https://github.com/cs3org/reva/pull/5741

 * Enhancement #5771: Replace backup_job_id with backup_name in projects table

   The backup_name column stores the project name plus a salt, used to identify the project in the
   backup system.

   https://github.com/cs3org/reva/pull/5771

 * Enhancement #5841: Fail over to another node when a peer cannot be reached

   A client resolved through the service registry used to be pinned to the node it resolved at, so a
   call to a peer that had died failed even when other nodes of that service were serving. The caller
   had to know about nodes to do anything about it.

   Clients now resolve a node per call. A call that fails because its node could not be reached is
   sent to another node of the same service, up to three distinct nodes, within the caller's
   deadline. Whether a call may be replayed is decided from what the connection was doing when it
   broke: a connection that was not yet ready never handed the request over, so any call is moved on,
   while a connection that broke mid-flight only replays reads, never a write.

   A node that could not be reached is then passed over for thirty seconds, so the calls that follow
   start somewhere else instead of rediscovering the same dead peer, and is picked up again as soon
   as it answers. This is tracked per process, because a node re-registers itself as ready on every
   heartbeat: the failure worth reacting to — a peer that is healthy to the registry but
   unreachable from here — is one only the calling process can observe.

   None of this is visible to the caller, which still asks for a client and makes a call.

   https://github.com/cs3org/reva/pull/5841

 * Enhancement #5665: Add a service registry for service discovery

   Reva services now find each other through a registry instead of hard-coded addresses. Each
   revad self-registers its loaded services, and inter-service calls resolve a peer by kind
   through a Clients resolver, with no address passed or seen by the caller. The HTTP data path
   (data gateway and data provider) is discovered the same way through a generic endpoint lookup,
   with the data provider selected by mount_id affinity. The registry is backed by an in-memory
   store by default or by NATS JetStream for a shared fleet view, with a liveness state machine that
   skips quiet or dead nodes. The per-peer address keys (gatewaysvc, the *_svc keys,
   [shared].datagateway, and data_server_url) are removed from configuration.

   https://github.com/cs3org/reva/pull/5665

 * Enhancement #5746: Implement shallow reconcile job

   https://github.com/cs3org/reva/pull/5746

 * Enhancement #5780: Add a takeout service to export a user's home directory

   Users can now request an export of their whole home directory, which runs as a background job
   that streams their files into one or more archives and emails the download links once ready.
   Expired exports are removed by a periodic cleanup job. The archiver service now shares its core
   with the takeout job through the new bundler package.

   https://github.com/cs3org/reva/pull/5780

 * Enhancement #5801: Cache for app passwords used by sync client

   This ensures we dont need to do a DB call for every connection of a sync client

   https://github.com/cs3org/reva/pull/5801


