---
title: "OCM webapp offers"
linkTitle: "OCM webapp offers"
weight: 40
---

# Outbound OCM webapp offers

Outbound webapp offers are disabled by default. Existing WebDAV offers
continue to work. Enable the provider-level offer only after the receiving
deployment advertises the required OCM capabilities and has a working token
endpoint.

## Configuration

Set these keys under `grpc.services.ocmshareprovider`:

```toml
[grpc.services.ocmshareprovider]
offer_webapp = true
webapp_name = "JupyterLab"
webapp_endpoint = "https://provider.example/services/ocm/open"
```

`offer_webapp` defaults to `false`. When it is disabled, `webapp_name` and
`webapp_endpoint` may be omitted. When it is enabled:

- `webapp_name` must not be empty or whitespace-only.
- `webapp_endpoint` must be an absolute http or https URL with a non-empty
  host and no userinfo.
- The configured application name is preserved exactly on offered shares.

Invalid enabled configuration fails startup with a bad-request error rather
than silently offering an incomplete webapp.

## Receiver configuration

Token exchange identifies the receiving provider by its host-only
`provider_domain`. ScienceMesh and every `ocmreceived` driver must share
this host-only FQDN.

```toml
[http.services.sciencemesh]
provider_domain = "receiver.example"

[grpc.services.storageprovider.drivers.ocmreceived]
provider_domain = "receiver.example"

[http.services.dataprovider.drivers.ocmreceived]
provider_domain = "receiver.example"
```

## Offer gate

The provider offers a webapp method only when all four conditions hold:

1. `offer_webapp` is enabled.
2. The request includes a webapp access candidate.
3. The remote resource advertises `webapp-receive` with the `blank` target.
4. The remote provider advertises `exchange-token`.

Otherwise, webapp methods are removed while WebDAV and other requested methods
remain. The provider records one fixed omission reason that carries no DTO,
URL, application name, secret, or token. The gate is evaluated before ignored
webapp candidates are validated, and the provider never synthesizes a
candidate, adds a file or folder branch, requires an iframe, or uses the
well-known `enable_webapp` setting as the offer gate.

When the gate holds, the provider rejects nil or typed-nil webapp options and
rejects two or more eligible webapp candidates so a JSON map overwrite cannot
select one. The failure happens before the share is created or stored.

## Opener endpoint

`webapp_endpoint` is the complete opener endpoint:

```text
https://provider.example/services/ocm/open
```

The provider does not append `/lab` or a share opaque ID. The opener obtains
share identity from the access token.

## Requirement normalization

Offered webapp requirements are normalized before the share is wired or
persisted, so the stored share and the outgoing request carry the same
methods.

- Empty webapp requirements receive an independent copy of the default
  webapp requirements, which contain `must-exchange-token`.
- Supplied non-empty webapp requirements must contain `must-exchange-token`
  and only known, non-blank, unpadded, unique values. Their order is
  preserved.
- An outbound `must-use-mfa` request is recognized and never erased; the
  receiving server's permanent MFA rejection policy is not applied to
  sending.
- For mixed webapp and WebDAV offers, an empty retained WebDAV requirements
  list defaults to a separate exchange-requirement copy. Supplied non-empty
  WebDAV lists must pass their vocabulary and agree as sets with the webapp
  list; conflicts are rejected rather than weakening the requirements. A
  requested webapp MFA requirement cannot combine with the current WebDAV
  vocabulary.

Normalization errors return a gRPC invalid-argument error with fixed safe
text and perform no remote POST or store call.

## Deployment checks

The well-known service's `enable_webapp` setting controls discovery
advertisements only. It is not the provider's outbound offer gate.
`enable_code_flow` advertises token exchange support. Before enabling offers,
check the effective local advertisements and the token endpoint in the
deployment. Also verify the hub spawner's `default_url` matches the selected
application deployment. For the JupyterLab deployment, that value is `/lab`;
`/lab` remains a spawner setting and is not part of `webapp_endpoint`.

## Received webapp shares

Incoming webapp offers are screened before discovery, conversion, or
persistence. The same requirement policy is used for the generic protocol
validator and the received-webapp check:

- `must-exchange-token` is mandatory.
- Blank, padded, and unknown requirements are rejected.
- `must-use-mfa` is recognized and permanently rejected. This receiver does
  not check or claim a session proof. The rejection text is: "protocol
  webapp requirement must-use-mfa cannot be satisfied by this receiver".

A webapp is kept only when discovery for that share shows `exchange-token`
and a usable token endpoint. The endpoint may be absolute http or https, or
a path-relative reference resolved against the discovery endpoint. A missing
capability or a bad endpoint is rejected before the share is stored.

The webapp URI that is stored must already be absolute http or https, with a
hostname and no userinfo. Relative, network-path, and malformed webapp URIs
are rejected. They are not resolved against the sender. WebDAV relative
paths are unchanged. Accepting an absolute http URI at ingest does not
change launch: open-in-app still requires https for the application URI and
the token endpoint.

Empty and padded `appName` values are stored as received. The only receive
target this service can open is `blank`.

Parser failures return a fixed invalid-request response. The response and
its log line do not include the request body or the decoder's cause.

## Discovery gates

OCM discovery advertises `apiVersion` 1.4.0. That version is the document
this service publishes. It is not proof that every OCM 1.4 behavior is
implemented. Every User-Agent receives that same document.

`enable_webapp` is discovery-only and is distinct from `offer_webapp`.
An invalid or empty discovery base leaves discovery disabled and publishes
no receive targets. Otherwise:

- Webapp off and code flow off: no webapp send, no webapp receive, and no
  token endpoint.
- Webapp on and code flow off: no webapp send, webapp receive with `blank`,
  and no token endpoint.
- Webapp off and code flow on: no webapp send, no webapp receive, and a
  validated token endpoint with `exchange-token`.
- Webapp on and code flow on: webapp send, webapp receive with `blank`, and
  a validated token endpoint with `exchange-token`.

The token endpoint is advertised only when it can be built and the result is
an absolute http or https URL with a hostname and no userinfo. The receive
targets in that document are the same targets published for local share
acceptance. A nil local override uses those published targets. An explicit
empty override accepts no webapp share.

## Specification scope

This service does not implement the whole webapp profile that an
`apiVersion` of 1.4.0 might suggest:

- `must-use-mfa` is rejected rather than satisfied.
- `iframe` is not a usable receive target.
- Discovery does not advertise `enforce-mfa`, HTTP request signatures, or
  `jwksUri`.
- There is no second launch-policy check beyond the https rule used by
  open-in-app.
- `expired_session_redirect_uri` session-redirect completion is not
  implemented; this service does not perform or advertise session-redirect
  handoff.
