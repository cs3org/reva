Bugfix: Make Jaeger agent usable

Previously, you could not use tracing with jaeger agent because the tracing connector is always used instead of the tracing endpoint.

This PR removes the defaults for collector and tracing endpoint. 

https://github.com/cs3org/reva/pull/1379
