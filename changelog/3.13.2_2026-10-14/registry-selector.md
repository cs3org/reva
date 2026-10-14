Enhancement: configurable registry selector

`selector` in `[shared.registry]` chooses local (the default: a node on the same host, else a random one), first, random or roundrobin, and a service's nodes are listed in id order.

https://github.com/cs3org/reva/pull/5883
