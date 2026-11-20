Bugfix: use provider FQDN for token exchange

Received WebDAV and upload token exchanges now send the configured
`provider_domain` as `client_id` instead of the user or grantee IdP, matching
the ScienceMesh launch. Every `ocmreceived` driver now requires
`provider_domain`, validated as a host-only DNS name with at least two labels;
an empty or invalid value, including `host:port`, fails startup.

https://github.com/cs3org/reva/pull/5851
