Enhancement: appproviders: pass other query parameters as Opaque

This allows to send any other HTTP query parameter passed to /app/open
to the underlying appprovider drivers via GRPC

https://github.com/cs3org/reva/pull/3502
