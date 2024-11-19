Bugfix: Avoid gateway panics

The gateway would panic if there is a missing user in the context. Now it errors instead.

https://github.com/cs3org/reva/issues/4953
