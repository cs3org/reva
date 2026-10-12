---
title: "OCM Network Policy"
linkTitle: "OCM Network Policy"
weight: 50
description: Operator controls for outbound OCM HTTP transports
---

Reva's OCM integration makes outbound HTTP calls to other providers. Those
calls split into two trust classes:

- Trusted calls go to operator-configured endpoints, such as a configured
  ScienceMesh directory service. They use a trusted HTTP client that honors
  HTTP proxy environment variables and dials any configured address.
- Public-only calls go to endpoints named by an untrusted caller, such as a
request-supplied `/discover` domain or a provider URL taken from an unsigned
directory response. They use a public-only HTTP client that dials public
addresses only and rejects private, loopback, link-local, CGNAT, NAT64, and
other special-purpose addresses before the connection is opened.

This document describes the operator controls that shape those transports.
It does not change provider authorization, token validation, or response
size limits.

## Independent controls

The address policy, TLS verification, loopback exception, and environment
proxy are independent. Turning one on never turns another on:

- `allowed_federation_cidrs` admits explicit private addresses only. Directory
  fetches are not granted any CIDR exception; only listed providers and
  `/discover` requests are subject to the public-only policy with the
  configured exception. It grants no TLS bypass and no loopback. Loopback
  keeps its own flag and stays off for ScienceMesh and the open authorizer.
- `insecure` skips TLS certificate verification only. It grants no address
  access. A private destination still requires an explicit CIDR entry.
- `allow_loopback_federation` admits loopback IP destinations only, and only
  for the receiver surfaces that carry it. It does not accept hostname
  aliases such as `localhost` or any other private address.
- `*_use_env_proxy` is the explicit environment-proxy opt-in for public-only
  clients. Direct mode (the default) is required to enforce destination
  CIDRs at the actual peer connection. When a proxy is selected, the guard
  sees the proxy hop, not the OCM target; CIDRs do not make a selected proxy
  enforce target ranges.

## Explicit federation CIDR exception

`allowed_federation_cidrs` is an empty-by-default array of canonical CIDR
strings. Each entry must be wholly inside RFC 1918 IPv4
(`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) or IPv6 ULA (`fc00::/7`).
Host bits must be cleared; `10.1.2.0/24` is accepted, `10.1.2.3/8` is not.
One-host entries use `/32` (IPv4) or `/128` (IPv6). The list is parsed once at
startup; any invalid entry aborts initialization with a fixed error. There is
no partial acceptance, no trimming, and no fallback to a trusted client.

The exception authorizes addresses, not DNS identities, ports, tenants, or
credentials. Any host and port in a selected range becomes reachable by that
consumer. Configure a narrow subnet or single host; no broad private root is
installed by default.

Example for an operator-controlled network (not production defaults):

```toml
allowed_federation_cidrs = ["10.197.228.0/24", "fd42:8c6d:7a10:23::/64"]
```

## Configuration tables

Each service copies its existing timeout and TLS keys into the shared runtime
transport config. The new `allowed_federation_cidrs` key uses the same spelling
and empty default everywhere it appears. Do not rename existing keys.

### [http.services.ocm]

Inbound OCM HTTP service. Public-only; `ocm_client_use_env_proxy` is the
explicit proxy opt-in. `allow_loopback_federation` is the receiver loopback
exception used by integration fixtures.

| Key | Default | Use |
|---|---|---|
| `ocm_client_timeout` | 10 | Request timeout, integer seconds |
| `ocm_client_insecure` | false | Skip TLS verification |
| `ocm_client_use_env_proxy` | false | Environment-proxy opt-in |
| `allow_loopback_federation` | false | Loopback IP exception |
| `allowed_federation_cidrs` | [] | Explicit private-network exception |

### [grpc.services.storageprovider.drivers.ocmreceived]

Received OCM storage driver. Public-only; `ocm_use_env_proxy` is the explicit
proxy opt-in.

| Key | Default | Use |
|---|---|---|
| `ocm_timeout` | 10 | Request timeout, integer seconds |
| `ocm_insecure` | false | Skip TLS verification |
| `ocm_use_env_proxy` | false | Environment-proxy opt-in |
| `allow_loopback_federation` | false | Loopback IP exception |
| `allowed_federation_cidrs` | [] | Explicit private-network exception |

### [http.services.dataprovider.drivers.ocmreceived]

Same driver implementation as the gRPC received storage driver; the same keys
and wiring apply.

| Key | Default | Use |
|---|---|---|
| `ocm_timeout` | 10 | Request timeout, integer seconds |
| `ocm_insecure` | false | Skip TLS verification |
| `ocm_use_env_proxy` | false | Environment-proxy opt-in |
| `allow_loopback_federation` | false | Loopback IP exception |
| `allowed_federation_cidrs` | [] | Explicit private-network exception |

### [http.services.sciencemesh]

ScienceMesh WAYF handler. Operator-configured directory fetches stay trusted.
Listed providers and request-supplied `/discover` domains use the public-only
client with the configured exception. Loopback and environment proxy stay off.

| Key | Default | Use |
|---|---|---|
| `ocm_client_timeout` | 10 | Request timeout, integer seconds |
| `ocm_client_insecure` | false | Skip TLS verification |
| `allowed_federation_cidrs` | [] | Explicit private-network exception |

### [grpc.services.ocmproviderauthorizer.drivers.open]

Open provider authorizer. Public-only first-hop discovery; no local
two-provider topology. Loopback stays off. Configure this key only when using
the open driver; do not switch a configured `json` or `mentix` driver to `open`
to get this control.

| Key | Default | Use |
|---|---|---|
| `insecure` | false | Skip TLS verification |
| `allowed_federation_cidrs` | [] | Explicit private-network exception |

## Defaults and equivalence

An empty or missing `allowed_federation_cidrs` is equivalent to the zero
policy: no private exceptions. Ordinary public destinations remain permitted
and unchanged. Existing default-deny behavior, response size limit, and TLS
1.2 floor are fixed and are not configurable here.

## Startup and restart

The CIDR list is parsed once during service initialization, before any
directory fetch. Invalid CIDRs abort initialization even when the directory
list is empty. Restart the process after changing the list, the timeout, the
TLS keys, or (for opt-in services) the proxy environment variables: Go reads
`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` once per process on the first
`ProxyFromEnvironment` call, and the clients are built once at startup.
