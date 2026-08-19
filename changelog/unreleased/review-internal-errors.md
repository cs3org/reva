Change: return tailored grpc error codes instead of always INTERNAL

Many grpc handlers wrapped every error in status.NewInternal, hiding the real
code from callers. They now map known errtypes (not found, permission denied,
already exists, invalid argument, ...) to the matching grpc status code through
a fixed status.NewStatusFromErrType, and the gateway share handlers propagate
the downstream status instead of flattening it.

https://github.com/cs3org/reva/pull/5781
https://github.com/cs3org/reva/issues/4915
