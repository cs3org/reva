Enhancement: mint view only token for open in app requests

When a view only mode is requested for open in app requests the gateway now mints a view only token scoped to the requested resource.
This token can be used by trusted app providers to download the resource even if the user has no download permission.

https://github.com/cs3org/reva/pull/4686
