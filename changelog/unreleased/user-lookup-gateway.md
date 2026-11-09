Enhancement: Lookup grantee user type in the grpc gateway layer

Previously this has been done in the HTTP layer, which means
requests going straight to the GRPC layer will fail if there's
no user type set

https://github.com/cs3org/reva/pull/6000