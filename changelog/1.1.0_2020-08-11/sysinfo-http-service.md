Enhancement: System information HTTP service

This service exposes system information via an HTTP endpoint. This currently only includes Reva version information but can be extended easily. The information are exposed in the form of Prometheus metrics so that we can gather these in a streamlined way.

https://github.com/cs3org/reva/pull/1037
