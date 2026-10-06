Enhancement: Add a service registry for service discovery

Reva services now find each other through a registry instead of hard-coded
addresses. Each revad self-registers its loaded services, and inter-service
calls resolve a peer by kind through a Clients resolver, with no address passed
or seen by the caller. The HTTP data path (data gateway and data provider) is
discovered the same way through a generic endpoint lookup, with the data
provider selected by mount_id affinity. The registry is backed by an in-memory
store by default or by NATS JetStream for a shared fleet view, with a liveness
state machine that skips quiet or dead nodes. The per-peer address keys
(gatewaysvc, the *_svc keys, [shared].datagateway, and data_server_url) are
removed from configuration.

https://github.com/cs3org/reva/pull/5665
