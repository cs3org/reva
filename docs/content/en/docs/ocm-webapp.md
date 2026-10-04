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

## Receiver network policy

Received-share discovery, WebDAV access and token exchange use the configured
public-only transports described in
[OCM network policy](../ocm-network-policy/). ScienceMesh `/open-in-app` uses
its own service configuration for both sender discovery and token exchange.
Private federation requires the same intended `allowed_federation_cidrs` on
each participating receiver service and each `ocmreceived` driver instance;
configuring one does not configure the others.

An empty list denies private destinations. CIDR exceptions do not disable TLS
verification or the TLS 1.2 floor and do not admit plain HTTP or loopback.
ScienceMesh keeps loopback and environment proxy disabled. Receiver OCM and
`ocmreceived` proxy options are separate opt-ins; direct mode is required when
the dial guard must enforce the destination CIDR. A selected proxy changes
the connection the guard sees and is not proof of target-range enforcement.

The browser opens the returned application URL itself. Server network policy
applies to Reva's discovery, token exchange and WebDAV requests; it is not a
browser application-origin allowlist. Open-in-app still requires HTTPS for
the application URI and token endpoint.

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
  receiving server's MFA admission policy is not applied to
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
