Enhancement: System information metrics cleanup

The system information metrics are now based on OpenCensus instead of the Prometheus client library. Furthermore, its initialization was moved out of the Prometheus HTTP service to keep things clean.

https://github.com/cs3org/reva/pull/1114
