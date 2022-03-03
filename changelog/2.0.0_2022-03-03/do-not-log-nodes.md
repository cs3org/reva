Enhancement: do not log whole nodes

It turns out that logging whole node objects is very expensive and also
spams the logs quite a bit. Instead we just log the node ID now.

https://github.com/cs3org/reva/pull/2463
