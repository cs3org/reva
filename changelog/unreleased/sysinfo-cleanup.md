Enhancement: System information metrics now use OpenCensus and were moved out of the Prometheus service

The system information metrics are now based on OpenCensus instead of the Prometheus client library. Furthermore, its initialization was moved out of the Prometheus HTTP service to keep things clean.

https://github.com/cs3org/reva/pull/1114
