Bugfix: A grpc or http block with no services is no longer rejected

Declaring a block that carries only settings, such as a [grpc] with nothing but
control_address, failed at startup with "grpc.services must be a map". The
services key was read with a type assertion, which also fails when the key is
simply absent. A process whose only grpc listener is its control channel can
now start.

https://github.com/cs3org/reva/pull/5840