Enhancement: request counting middleware

We added a request counting `prometheus` HTTP middleware and GRPC interceptor that can be configured with a `namespace` and `subsystem` to count the number of requests.

https://github.com/cs3org/reva/pull/3229
