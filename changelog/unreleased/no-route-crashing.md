Enhancement: ignore unknown routes

Currently, the gateway crashes with a fatal error if it encounters any unknown routes in the routing table. Instead, we log the error and ignore the routes, which should make upgrades in the routing table easier.

https://github.com/cs3org/reva/pull/5205