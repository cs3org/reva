# Outbound OCM webapp offers

Outbound webapp offers are disabled by default. Existing WebDAV offers
continue to work. Enable the provider-level offer only after the receiving
deployment advertises the required OCM capabilities and has a working token
endpoint.

## Configuration

Set these keys under `grpc.services.ocmshareprovider`:

```toml
[grpc.services.ocmshareprovider]
offer_webapp = true
webapp_name = "JupyterLab"
webapp_endpoint = "https://provider.example/services/ocm/open"
```

`offer_webapp` defaults to `false`. When it is disabled, `webapp_name` and
`webapp_endpoint` may be omitted. When it is enabled:

- `webapp_name` must not be empty or whitespace-only.
- `webapp_endpoint` must be an absolute URL with a non-empty host.
- The configured application name is preserved exactly on offered shares.

The provider offers a webapp method only when the request includes a webapp
candidate, the remote resource advertises `webapp-receive` with the `blank`
target, and the remote provider advertises `exchange-token`. Otherwise,
webapp methods are removed while WebDAV and other requested methods remain.

## Opener endpoint

`webapp_endpoint` is the complete opener endpoint:

```text
https://provider.example/services/ocm/open
```

The provider does not append `/lab` or a share opaque ID. The opener obtains
share identity from the access token.

## Deployment checks

The well-known service's `enable_webapp` setting controls discovery
advertisements only. It is not the provider's outbound offer gate.
`enable_code_flow` advertises token exchange support. Before enabling offers,
check the effective local advertisements and the token endpoint in the
deployment. Also verify the hub spawner's `default_url` matches the selected
application deployment. For the JupyterLab deployment, that value is `/lab`;
`/lab` remains a spawner setting and is not part of `webapp_endpoint`.
