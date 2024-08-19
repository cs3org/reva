Bugfix: Fix deleting space shares

We no longer check if a share is an ocm sharee if listng ocm shares has been disabled anyway. This allows unsharing space shares.

https://github.com/cs3org/reva/pull/4651
