Enhancement: Return full public link url in GRPC layer

The token was the only thing that was returned when creating
a public link in the GRPC layer, as the full public link was constructed
in the HTTP layer. This is fixed with this change as it causes issues
for the GRPC clients.

https://github.com/cs3org/reva/pull/6027