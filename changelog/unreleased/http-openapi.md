Enhancement: Describe the HTTP API as OpenAPI

The new `openapi` service serves an OpenAPI 3.2 document built from the routes
the services declared, and a page that browses it. Every HTTP service in the
fleet describes its own routes through the new `openapi` invocation, and the
service merges them through the Admin API. Without the Admin API, it describes
the routes of its own server. The credentials each server accepts come from
the configuration of its auth middleware. The page loads a pinned Swagger UI
release and checks it against its Subresource Integrity hash.

https://github.com/cs3org/reva/pull/5910
