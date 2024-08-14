Enhancement: Implement OCM well-known endpoint

The `wellknown` service now implements the `/.well-known/ocm` endpoint for OCM discovery. The unused endpoints for openid connect and webfinger have been removed. This aligns the wellknown implementation with the master branch.

https://github.com/cs3org/reva/pull/4809
