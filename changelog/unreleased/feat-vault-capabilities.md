Enhancement: Add vault=true query parameter support to the capabilities endpoint

The OCS capabilities handler now accepts a `vault=true` query parameter.
When `CapabilitiesVault.Enabled` is true and the parameter is present, the
endpoint returns a modified capabilities response where public sharing
(`files_sharing.public.enabled`) and federation sharing
(`files_sharing.federation.outgoing` / `incoming`) are disabled.
All other capabilities remain unchanged.

https://github.com/owncloud/reva/pull/584
