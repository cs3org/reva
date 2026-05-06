Enhancement: Add vault storage provider and MFA propagation

Added a dedicated vault storage provider. The storage registry now filters out
vault spaces when no explicit `storage_id` is provided and routes requests with
a matching storage ID exclusively to the vault provider.

A new `autoprop-mfa-authenticated` gRPC metadata key (`MFAOutgoingHeader` in
`pkg/ctx/mfactx.go`) propagates MFA status across service boundaries using the
auto-propagation interceptor. The HTTP auth interceptor reads the
`X-Multi-Factor-Authentication` header and injects the gRPC metadata
accordingly. The WOPI/collaboration service embeds MFA status in the WOPI token
(`HasMFA`) and sets the header on outgoing HTTP requests to the data server.

Decomposedfs now accepts a configurable `consumer_group` for the events
consumer (defaults to `dcfs`), allowing multiple storage instances to consume
events independently. Events carrying a `ResourceID.StorageId` are filtered so
that each storage instance only handles events for its own mount. The
`StorageId` is now included in the upload-finished event to enable this
routing.

WebDAV copy/move operations that would transfer files out of the vault into
non-vault storage are blocked with a permission error.

The redundant `getStorageProviderClient` wrapper in the OCS sharing handler was
removed in favour of a direct `pool.GetStorageProviderServiceClient` call.

https://github.com/owncloud/reva/pull/559
