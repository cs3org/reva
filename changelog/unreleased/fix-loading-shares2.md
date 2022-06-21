Bugfix: Do not close share dump channels

We no longer close the channels when dumping shares, it's the responsibility of the caller.

https://github.com/cs3org/reva/pull/2996
