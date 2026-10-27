---
title: Configuration
description: The structure of revad.toml, and how to run services in several processes.
weight: 20
---

`revad` reads one TOML file, given with `-c`. Check a file without starting the server with `revad -t -c revad.toml`.

## Top-level tables

| Table | Contents |
| --- | --- |
| `[shared]` | Settings that many services read: `jwt_secret`, `gatewaysvc`, `datagateway`, `blocked_users`, the service `registry`, `mtls`, and the database connection. A service uses these values when it does not set its own. |
| `[log]` | `output`, `mode` (`console` or `json`), `level`, and `tail`. |
| `[grpc]` | The gRPC listener: `address`, `network`, TLS and mTLS settings. |
| `[grpc.services.<name>]` | One table per [gRPC service](/docs/components/grpc-services/). |
| `[grpc.interceptors.<name>]` | [gRPC interceptors](/docs/components/grpc-interceptors/). |
| `[http]` | The HTTP listener: `address`, `certfile`, `keyfile`. |
| `[http.services.<name>]` | One table per [HTTP service](/docs/components/http-services/). |
| `[http.middlewares.<name>]` | [HTTP middlewares](/docs/components/http-middlewares/), such as `auth` and `cors`. |
| `[serverless.services.<name>]` | [Background services](/docs/components/serverless-services/) without a listener. |
| `[vars]` | Your own values, for use in templates. |

Each component page lists every setting of that component, with its type, default, and a link to the source.

## Drivers

A service that uses a driver has a `driver` setting and one table per driver under `drivers`:

```toml
[grpc.services.storageprovider]
driver = "eos"

[grpc.services.storageprovider.drivers.eos]
master_url = "root://eos.example.org"
namespace = "/eos/user/"
```

The [driver families](/docs/components/drivers/) list the drivers each service can use.

## Several instances of one service

Use a TOML array of tables to run a service more than once in one process, each on its own address:

```toml
[[grpc.services.storageprovider]]
address = "0.0.0.0:17000"
driver = "eos"
mount_id = "eos-user"

[[grpc.services.storageprovider]]
address = "0.0.0.0:17001"
driver = "localhome"
mount_id = "scratch"
```

## Templates and variables

String values can refer to `[vars]` and to the addresses of other services in the same file:

```toml
[vars]
db_password = "secret"

[shared]
gatewaysvc = "{{ grpc.services.gateway.address }}"

[grpc.services.usershareprovider.drivers.sql]
db_password = "{{ vars.db_password }}"
```

## Splitting services across processes

Put the services in several files and start one `revad` per file. Point `gatewaysvc` in `[shared]` at the gateway, and use the same `jwt_secret` everywhere. The processes register in the service registry, set in `[shared.registry]`. The default registry is in memory and only works inside one process; use a shared back end, such as [NATS](/docs/components/drivers/service-registries/nats/), across processes.

For development, `revad -dev-dir <folder>` starts one process per TOML file in a folder.
