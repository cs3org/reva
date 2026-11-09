
---
title: "v3.15.1"
linkTitle: "v3.15.1"
weight: 999549
description: >
  Changelog for Reva v3.15.1 (2026-11-09)
---

Changelog for reva 3.15.1 (2026-11-09)
=======================================

The following sections list the changes in reva 3.15.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Sec #5983: Clean path in lw scope, drop archiver paths
 * Sec #5981: Fix possible select-level sql injection in share db
 * Sec #5980: Add extra permission checks on gateway List(Public)Shares
 * Sec #5982: Do not take modbits into account for ext accounts
 * Sec #5864: Require an explicit shared JWT secret
 * Fix #5988: Do not report leading zeros in adler32 checksum
 * Fix #5990: Report adler32 checksum in PascalCase
 * Fix #5974: Fix broken single file shares
 * Fix #5969: Fix several problems in the cephmount user threads
 * Fix #5992: Nilpointer exception when owner is unknown
 * Fix #5933: Expose resolution of /home via the OCS APIs
 * Fix #5987: Return the correct status when a PUT cannot start an upload
 * Fix #5885: Guard share and invite authentication records
 * Fix #5851: Use provider FQDN for token exchange
 * Fix #5855: Validate received webapp requirements
 * Fix #5856: Add client_id claim to minted OCM token
 * Fix #5975: PROPFIND for single file should have 2 entries
 * Fix #5945: Let gRPC services inherit the network from the gRPC defaults
 * Chg #5909: Remove the ScienceMesh open-in-app route
 * Enh #5970: Limit the number of cephmount user threads
 * Enh #5972: Run the requests of one cephmount user in parallel
 * Enh #5910: Describe the HTTP API as OpenAPI
 * Enh #5989: Support If-None-Match on PUT
 * Enh #5852: Add Graph webapp metadata carrier
 * Enh #5907: Exchange received share tokens when offered
 * Enh #5854: Gate outbound webapp offers
 * Enh #5853: Expose received webapp metadata
 * Enh #5850: Add safe received webapp launch
 * Enh #5851: Discover the OCM token endpoint at the sender
 * Enh #5906: Add served webapp launcher page
 * Enh #5968: Ocm requests are not signed with http signatures
 * Enh #5850: Opt ScienceMesh clients into the env proxy

Details
-------

 * Security #5983: Clean path in lw scope, drop archiver paths

   https://github.com/cs3org/reva/pull/5983

 * Security #5981: Fix possible select-level sql injection in share db

   https://github.com/cs3org/reva/pull/5981

 * Security #5980: Add extra permission checks on gateway List(Public)Shares

   https://github.com/cs3org/reva/pull/5980

 * Security #5982: Do not take modbits into account for ext accounts

   https://github.com/cs3org/reva/pull/5982

 * Security #5864: Require an explicit shared JWT secret

   Reva no longer falls back to a built-in JWT signing key. Deployments relying on the shared
   secret must configure `shared.jwt_secret` with a high-entropy value and share it only among
   the Reva services in the same deployment. If neither a shared nor token-manager-specific
   secret is configured, JWT manager creation now fails with an actionable error. Missing
   configuration can no longer select a publicly known signing key.

   https://github.com/cs3org/reva/pull/5864

 * Bugfix #5988: Do not report leading zeros in adler32 checksum

   The owncloud sync client does an exact comparision, and it does not expect a leading zero,
   leading it to reject the file

   https://github.com/cs3org/reva/pull/5988

 * Bugfix #5990: Report adler32 checksum in PascalCase

   It used to be reported in upper case, but the NextCloud client does not accept that

   https://github.com/cs3org/reva/pull/5990

 * Bugfix #5974: Fix broken single file shares

   No access to the parent folder should not result in a 403

   https://github.com/cs3org/reva/pull/5974

 * Bugfix #5969: Fix several problems in the cephmount user threads

   - Cleanup stopped a user thread while a long request still ran on it. - A request whose caller had
   stopped waiting still ran later. - An upload copied its data on the user thread, so a slow client
   blocked the other requests of that user. - Uploads now also return the error from closing the
   file. - A data race on the thread ID is also fixed.

   https://github.com/cs3org/reva/pull/5969

 * Bugfix #5992: Nilpointer exception when owner is unknown

   When the owner of a file is unknown (e.g. deleted account) the cephmount driver had a nil pointer
   exception, this is now fixed.

   https://github.com/cs3org/reva/pull/5992

 * Bugfix #5933: Expose resolution of /home via the OCS APIs

   https://github.com/cs3org/reva/pull/5933

 * Bugfix #5987: Return the correct status when a PUT cannot start an upload

   When a gateway call returned a gRPC error, ocdav always sent a 500. For example, an external
   account that lost the editor role on a share got a 500 when it saved the file, and ownCloud Web
   showed "An error occurred" with no details. All ocdav handlers now converts the gRPC code to the
   matching HTTP status, so this case gives a 403 and the web client shows that the user cannot save
   the file.

   https://github.com/cs3org/reva/pull/5987

 * Bugfix #5885: Guard share and invite authentication records

   Reject malformed share records, grantees, creators and access methods before direct-secret
   authentication. Validate invitation requests, context identities, stored records and
   successful repository results, and fix the memory repository's first-insert and
   identity-key handling.

   JSON invitation updates replace the data file before publishing the live model; failed
   persistence reports an error without changing that model. Memory invitation listings omit
   elapsed tokens while retaining storage and listing of tokens without expiration. Redemption
   still requires a valid expiration.

   https://github.com/cs3org/reva/pull/5885

 * Bugfix #5851: Use provider FQDN for token exchange

   Received WebDAV and upload token exchanges now send the configured `provider_domain` as
   `client_id` instead of the user or grantee IdP, matching the ScienceMesh launch. Every
   `ocmreceived` driver now requires `provider_domain`, validated as a host-only DNS name with
   at least two labels; an empty or invalid value, including `host:port`, fails startup.

   https://github.com/cs3org/reva/pull/5851

 * Bugfix #5855: Validate received webapp requirements

   Incoming OCM webapp shares now require `must-exchange-token`, reject blank, padded, and
   unknown requirements, and refuse `must-use-mfa` unless `mfa_policy` is `off`, the same
   setting the launch path uses. A webapp share is stored only after sender discovery reports
   `exchange-token` with a usable token endpoint and the webapp URI is absolute http(s);
   relative URIs are rejected. OCM discovery now advertises `apiVersion` 1.4.0.

   The SQL store now keeps webapp permissions exact.

   https://github.com/cs3org/reva/pull/5855

 * Bugfix #5856: Add client_id claim to minted OCM token

   The OCM token endpoint now accepts only the `authorization_code` grant, and the form
   `client_id` must be the receiving provider FQDN and match the stored share recipient. Tokens
   minted through the code flow carry a `client_id` claim set to the share id; legacy
   direct-secret tokens do not. Verification re-derives that share id from the OCM share scope
   and rejects a token whose `client_id` claim disagrees. Tokens that omit the claim keep
   working.

   https://github.com/cs3org/reva/pull/5856

 * Bugfix #5975: PROPFIND for single file should have 2 entries

   https://github.com/cs3org/reva/pull/5975

 * Bugfix #5945: Let gRPC services inherit the network from the gRPC defaults

   https://github.com/cs3org/reva/pull/5945

 * Change #5909: Remove the ScienceMesh open-in-app route

   `POST /sciencemesh/open-in-app` is removed. It returned the exchanged access token to the
   browser as JSON. Received webapp shares now launch only through `POST
   /sciencemesh/webapp-launch`, which posts the token straight to the validated app URL. The
   lightweight account scope no longer allows the old path. The CS3 app provider `OpenInApp` call
   and the `reva open-in-app` CLI command are unchanged.

   https://github.com/cs3org/reva/pull/5909

 * Enhancement #5970: Limit the number of cephmount user threads

   The new option `max_user_threads` (default 1000) limits the threads of each cephmount
   driver. A thread counts until it exits. When the limit is reached, the idle thread that was used
   longest ago stops. If no thread is idle, the request fails.

   https://github.com/cs3org/reva/pull/5970

 * Enhancement #5972: Run the requests of one cephmount user in parallel

   All requests of one uid ran one at a time, so a slow request, such as the delete of a large folder,
   blocked the other requests of that user. All external accounts share one uid, so one slow
   request blocked all of them. Each uid now has up to `max_threads_per_user` threads (default
   4), and the uid of the external accounts up to `max_threads_external` (default 32).

   https://github.com/cs3org/reva/pull/5972

 * Enhancement #5910: Describe the HTTP API as OpenAPI

   The new `openapi` service serves an OpenAPI document built from the routes the services
   declared, and a page that browses it, so an endpoint cannot be served without being documented
   nor documented without being served. It reports the credentials a deployment accepts from the
   auth middleware's own configuration. The page loads a fixed release of its viewer, checked
   against its hash, and asks for a token when the document needs one.

   https://github.com/cs3org/reva/pull/5910

 * Enhancement #5989: Support If-None-Match on PUT

   A PUT with `If-None-Match: *` now gets a 412 when the file exists, so a client that creates a new
   file does not overwrite a file created in the meantime.

   https://github.com/cs3org/reva/pull/5989

 * Enhancement #5852: Add Graph webapp metadata carrier

   Graph sharedWithMe items can now carry received webapp metadata as an `@ocm.webApp` object on
   `remoteItem`, next to `permissions`, without changing the generated DriveItem types.
   Nothing sets the metadata yet, so responses are unchanged.

   https://github.com/cs3org/reva/pull/5852

 * Enhancement #5907: Exchange received share tokens when offered

   Received WebDAV shares without must-exchange-token now try OCM token exchange when sender
   discovery lists exchange-token and a token endpoint. The access token is cached until shortly
   before it expires. If discovery or the exchange fails, or the sender's WebDAV rejects the
   token, the driver falls back to the shared secret and remembers that failure for a short time. A
   policy violation from the sender is returned as an error, with no fallback. Shares marked
   must-exchange-token keep the strict path.

   https://github.com/cs3org/reva/pull/5907

 * Enhancement #5854: Gate outbound webapp offers

   Outbound OCM shares now offer webapp access only when `offer_webapp` is enabled (default
   off), the request carries a webapp candidate, and the remote advertises the `blank`
   webapp-receive target and `exchange-token`. The same access-method list is sent in
   `NewShare` and stored, using the configured `webapp_name` and `webapp_endpoint`. Invalid
   webapp requirements return InvalidArgument before any remote call, and an invalid enabled
   configuration fails startup.

   https://github.com/cs3org/reva/pull/5854

 * Enhancement #5853: Expose received webapp metadata

   Graph sharedWithMe now returns `remoteItem["@ocm.webApp"].appName` for a received OCM
   share that carries exactly one webapp protocol. The stored name is returned as is, including an
   empty string. Shares with no webapp protocol, or with more than one, omit the key.

   https://github.com/cs3org/reva/pull/5853

 * Enhancement #5850: Add safe received webapp launch

   ScienceMesh now launches received OCM webapp shares. The stored offer is validated before any
   dial (`must-exchange-token`, `blank` target, https app URI), then the sender is discovered
   and the shared secret is exchanged for an access token over the public-only OCM client, with the
   configured provider domain as `client_id`. The shared secret is never sent to the browser.
   `must-use-mfa` offers are refused unless `mfa_policy` in
   `[http.services.wellknown.ocmprovider]` is `off` (default `reject`); nothing is
   advertised in discovery. The launch uses the ScienceMesh timeout, TLS settings and explicit
   `allowed_federation_cidrs` for both discovery and token exchange. Without an exception,
   private addresses are refused. Loopback stays off. Environment proxy stays off unless
   `ocm_client_use_env_proxy` is set.

   https://github.com/cs3org/reva/pull/5850

 * Enhancement #5851: Discover the OCM token endpoint at the sender

   Received WebDAV code flow and ScienceMesh webapp launch now use sender-based discovery. Both
   look up the token endpoint at the share sender's provider, the creator idp, and do not fall back
   to the WebDAV or webapp URI origin. An empty or missing sender idp fails with not found and does
   not dial. The received-share discovery cache is keyed by that idp.

   https://github.com/cs3org/reva/pull/5851

 * Enhancement #5906: Add served webapp launcher page

   ScienceMesh now serves `POST /sciencemesh/webapp-launch`, a protected route that
   validates the received webapp share, exchanges the shared secret for an access token, and
   renders a small page that auto-submits a form posting `access_token` to the validated app URL.
   Nothing is stored on the server and no unprotected route is added. Every response sets
   `Content-Security-Policy` plus `Cache-Control: no-store`, `Referrer-Policy:
   no-referrer` and `X-Content-Type-Options: nosniff`. On the launcher page `form-action` is
   the exact validated app origin; internationalized hosts are posted to in their ASCII form, and
   a host that cannot be written as a CSP host source is refused before any token exchange. Error
   pages use `form-action 'none'` and carry fixed text only. An
   `expired_session_redirect_uri` is forwarded only when its origin is
   `https://<provider_domain>`; otherwise it is dropped.

   https://github.com/cs3org/reva/pull/5906

 * Enhancement #5968: Ocm requests are not signed with http signatures

   - `signing_key`: the private key of the server. reva publishes the public half, so that other
   servers can check our signatures. - reva signs these requests, and checks the signatures on
   them when it receives them: new shares (`/shares`), accepted invites (`/invite-accepted`)
   and code-flow token requests (`/token`). - `http_sig_mode`: `permissive` (the default)
   rejects unsigned requests only from servers that sign their own requests. `strict` rejects
   all unsigned requests. - `endpoint`: the public URL of the server, so that another server
   cannot reuse a signed request against us.

   https://github.com/cs3org/reva/pull/5968

 * Enhancement #5850: Opt ScienceMesh clients into the env proxy

   ScienceMesh WAYF listed-provider discovery, request-supplied `/discover`, and webapp
   launch discovery and token exchange honor `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` when
   `ocm_client_use_env_proxy` is set on `[http.services.sciencemesh]` (default false). The
   open provider authorizer accepts the same key on
   `[grpc.services.ocmproviderauthorizer.drivers.open]`. With the knob enabled, the guard
   classifies the proxy hop: an unlisted private proxy address is refused, and the CONNECT target
   is no longer separately classified. Direct mode (knob off) preserves target classification.

   https://github.com/cs3org/reva/pull/5850


