Bugfix: Enable TLS configuration for gRPC connections

Previously, one needed to setup their own proxy to use Reva with secured endpoints. Now, one can setup TLS for gRPC connections or use it insecurely depending on the chosen configuration.

See https://github.com/cs3org/reva/issues/1962 and https://github.com/cs3org/reva/issues/2216

