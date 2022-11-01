Bugfix: Fix lock acquiring timeout

We've increased the lock acquire timeout to 180ms max.
This makes the locking more tolerant and is useful when many mutations are to happen one after the other.

https://github.com/cs3org/reva/pull/3423
