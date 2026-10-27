---
title: CS3 APIs (gRPC)
linkTitle: CS3 APIs
description: The gRPC APIs that the reva services implement.
weight: 20
---

The [CS3 APIs](https://github.com/cs3org/cs3apis) are the protocol between all reva services. They are Protocol Buffers definitions, so you can write clients and services in any language that gRPC supports.

- [API reference on the Buf Schema Registry](https://buf.build/cs3org-buf/cs3apis)
- [Source of the definitions](https://github.com/cs3org/cs3apis)
- Generated client libraries: [Go](https://github.com/cs3org/go-cs3apis), [Python](https://github.com/cs3org/python-cs3apis), [JavaScript](https://github.com/cs3org/node-cs3apis)

## Main APIs

| API | Package | Served by |
| --- | --- | --- |
| Gateway | `cs3.gateway.v1beta1` | [gateway](/docs/components/grpc-services/gateway/) |
| Storage provider | `cs3.storage.provider.v1beta1` | [storageprovider](/docs/components/grpc-services/storageprovider/) |
| Storage registry | `cs3.storage.registry.v1beta1` | [storageregistry](/docs/components/grpc-services/storageregistry/) |
| Auth provider | `cs3.auth.provider.v1beta1` | [authprovider](/docs/components/grpc-services/authprovider/) |
| Users | `cs3.identity.user.v1beta1` | [userprovider](/docs/components/grpc-services/userprovider/) |
| Groups | `cs3.identity.group.v1beta1` | [groupprovider](/docs/components/grpc-services/groupprovider/) |
| Collaboration (shares) | `cs3.sharing.collaboration.v1beta1` | [usershareprovider](/docs/components/grpc-services/usershareprovider/) |
| Public links | `cs3.sharing.link.v1beta1` | [publicshareprovider](/docs/components/grpc-services/publicshareprovider/) |
| OCM | `cs3.sharing.ocm.v1beta1` | [ocmshareprovider](/docs/components/grpc-services/ocmshareprovider/) |
| App provider | `cs3.app.provider.v1beta1` | [appprovider](/docs/components/grpc-services/appprovider/) |

Clients call the gateway, which forwards to the providers. Run `revad` with `enable_reflection = true` in `[grpc]` to explore a running server with tools such as `grpcurl`.
