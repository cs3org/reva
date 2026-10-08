Bugfix: stop blaming nodes for their backends

A node relaying an Unavailable is no longer penalized, draining nodes no longer hide penalized ones, and an unverifiable token is answered with Unavailable.

https://github.com/cs3org/reva/pull/5881
