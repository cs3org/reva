Enhancement: configurable registry selector

A service's nodes are listed in id order, so the default first selector always picks the same node, and `selector` in `[shared.registry]` chooses first, random or roundrobin.

https://github.com/cs3org/reva/pull/5883
