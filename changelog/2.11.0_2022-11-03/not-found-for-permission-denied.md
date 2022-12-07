Enhancement: Do not leak existence of resources

We are now returning a not found error for more requests to not leak existence of spaces for users that do not have access to resources.

https://github.com/cs3org/reva/pull/3300
