Bugfix: Fix missing etag in shares jail

The shares jail can miss the etag if the first `receivedShare` is not accepted.

https://github.com/cs3org/reva/pull/4140
