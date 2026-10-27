---
title: Architecture
description: How a request travels from a client, through the gateway, to storage.
weight: 10
---

{{< diagram >}}

## Layers

**HTTP services** speak the protocols that clients already use: [WebDAV](/docs/components/http-services/ocdav/), the [OCS API](/docs/components/http-services/ocs/), a [Graph API](/docs/components/http-services/ocgraph/), and [Open Cloud Mesh](/docs/components/http-services/ocm/). They do not hold state. Each request becomes one or more CS3 API calls to the gateway.

**The [gateway](/docs/components/grpc-services/gateway/)** is the single entry point for CS3 API calls. It checks the caller's token, asks a registry where a resource or credential belongs, and forwards the call to the right provider. Clients such as the [reva CLI](/docs/getting-started/#4-use-the-reva-cli) call it directly.

**gRPC services** each own one domain: authentication, users, groups, storage, shares, apps, and OCM. They implement the [CS3 APIs](/docs/api/grpc/).

**Drivers** connect a service to a back end. The storage provider uses a [storage driver](/docs/components/drivers/storage-drivers/) such as EOS or CephFS. The auth provider uses an [auth manager](/docs/components/drivers/auth-managers/) such as LDAP or OIDC. A configuration file selects one driver per service.

**The data path is separate.** File contents do not pass through the gateway. The storage provider returns an upload or download URL with a short-lived transfer token. The client sends the bytes to the [data gateway](/docs/components/http-services/datagateway/), which forwards them to the [data provider](/docs/components/http-services/dataprovider/) next to the storage.

## Tokens

The HTTP auth middleware turns a password, an OIDC token, or another credential into a reva token, which is a JWT that the gateway signs. All later calls carry this token. Services trust the token and do not see the original credential.

## Processes

All services are plugins in one binary, `revad`. One process can run every service, as in the [quick start](/docs/getting-started/). A large deployment runs many processes, each with some services, and scales them separately. The processes find each other through the service registry. See [configuration](/docs/concepts/configuration/).

## Extending reva

Every service and driver registers itself by name when its package loads. To add a driver, implement the interface of its family and register it. External [plugins](https://github.com/cs3org/reva/wiki) can add drivers without changing reva.
