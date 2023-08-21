Bugfix: Return lock when requested

We did not explictly return the lock when it was requested. This lead to the lock only being included when no other metadata was requested. We fixed it by explictly returning the lock when requested.

https://github.com/cs3org/reva/pull/4107
