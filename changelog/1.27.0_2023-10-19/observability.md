Enhancement: Add better observability with metrics and traces

Adds prometheus collectors that can be registered dynamically and also 
refactors the http and grpc clients and servers to propage trace info.

https://github.com/cs3org/reva/pull/4166
