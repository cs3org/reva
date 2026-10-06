Bugfix: Make service names unique per transport 

Currently, multiple services with the same name (e.g. `preferences`), but using
different transports (`http` vs `grpc`) could conflict. This caused connections
to be randomly to either of the services. We now bind this to specific transports,
so a service is queried with a transport.

https://github.com/cs3org/reva/pull/5813