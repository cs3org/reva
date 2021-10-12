Bugfix: Map GRPC error codes to REVA errors

We've fixed the error return behaviour in the gateway which would return GRPC error codes from the auth middleware. Now it returns REVA errors which other parts of REVA are also able to understand.

https://github.com/cs3org/reva/pull/2140
