Bugfix: Use ocs permission objects in the reva GRPC client

There was a bug introduced by differing CS3APIs permission definitions
for the same role across services. This is a first step in making
all services use consistent definitions.

https://github.com/cs3org/reva/pull/2478