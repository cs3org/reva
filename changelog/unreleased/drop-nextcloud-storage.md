Change: Dropped support for Nextcloud as storage/user/auth provider

This PR removes the code to support Nextcloud as storage, user,
and auth provider.

This code was developed as part of the initial effort to put
in place the ScienceMesh, where the deployment model was to run
Reva at each site, including sites running Nextcloud where Reva
would be responsible for the OCM-based federation layer and
Nextcloud for all the rest.

Over the years, and especially during 2025-26, Nextcloud has
implemented all OCM-related capabilities natively, and this
interface is getting obsoleted. We have kept it until the
maintenance cost was negligible, but with the upcoming changes
on the OCM implementation, this is not sustainable any longer.

https://github.com/cs3org/reva/pull/5671

