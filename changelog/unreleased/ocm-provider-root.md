Enhancement: support full URL endpoints in ocm-provider

This patch enables a reva server to properly show any configured
endpoint route in all relevant properties exposed by /ocm-provider.
This allows reverse proxy configurations of the form https://server/route
to be supported for the OCM discovery mechanism.

https://github.com/cs3org/reva/pull/4189
