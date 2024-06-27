Enhancement: Respect service transport

The service registry now takes into account the service transport when creating grpc clients. This allows using `dns` and `unix` as the protocol in addition to `tcp`. `dns` will turn the gRPC client into a [Thick Client](https://grpc.io/blog/grpc-load-balancing/#thick-client) that can look up multiple endpoints via DNS. Furthermore, we enabled round robin load balancing for the [default transparent retry configuration of gRPC](https://grpc.io/docs/guides/retry/#retry-configuration).

https://github.com/cs3org/reva/pull/4744
