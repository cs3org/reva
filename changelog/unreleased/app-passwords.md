Enhancement: fine-grained app passwords

App passwords carry a client kind, permissions, optional roots and an expiry, enforced by a new `appauth` scope in the HTTP auth middleware and in every gRPC service, and are sent with basic auth or as a bearer token.
Users create them through the login flow, and administrators for any user through the Admin API.

https://github.com/cs3org/reva/pull/5894
