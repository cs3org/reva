Enhancement: ocm: support bearer token access

This PR adds support for accessing remote OCM 1.1 shares via bearer token,
as opposed to having the shared secret in the URL only.
In addition, the OCM client package is now part of the OCMD server package,
and the Discover methods have been all consolidated in one place.

https://github.com/cs3org/reva/pull/4670
