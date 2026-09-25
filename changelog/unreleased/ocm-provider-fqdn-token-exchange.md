Bugfix: use the receiving provider FQDN for token exchange

Received WebDAV and upload token exchanges now identify the receiving
provider; the ScienceMesh webapp launch already used the configured
host-only provider_domain in ScienceMesh and every ocmreceived
driver. An empty or invalid value fails startup.
