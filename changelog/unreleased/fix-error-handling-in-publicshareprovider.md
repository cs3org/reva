Bugfix: Properly handle not-found errors when getting a public share

We fixed a problem where not-found errors caused a hard error instead of a proper RPC error state.

https://github.com/cs3org/reva/pull/4057
