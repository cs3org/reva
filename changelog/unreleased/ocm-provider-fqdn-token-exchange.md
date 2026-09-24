Bugfix: use the receiving provider FQDN for token exchange

Received WebDAV and upload token exchange through the ocmreceived
driver previously sent a user or grantee IdP as client_id. Those
paths now send the configured receiving server. That value must be a
host-only DNS FQDN: no scheme, port, path, query, fragment, userinfo,
IP address, or single-label name. The configured spelling is kept.
Token exchange does not send an empty client_id. Received webapp
launch already used the configured ScienceMesh provider_domain and
remains unchanged.

The ocmreceived driver requires provider_domain. Set it to the same
host-only FQDN as ScienceMesh provider_domain for that receiver. Every
ocmreceived block needs the setting, including the HTTP data provider
when it uses this driver:

    [grpc.services.storageprovider.drivers.ocmreceived]
    provider_domain = "receiver.example.test"

An empty or invalid value fails driver and ScienceMesh startup.
