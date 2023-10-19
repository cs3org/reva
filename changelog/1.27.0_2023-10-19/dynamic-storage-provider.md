Enhancement: Dynamic storage provider

Add a new storage provider that can globally route to other providers. This
provider uses a routing table in the database containing `path` - `mountid`
pairs, and a mapping `mountid` - `address` in the config. It also support
rewriting paths for resolution (to enable more complex cases).

https://github.com/cs3org/reva/pull/4199
