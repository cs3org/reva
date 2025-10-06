Bugfix: disallow setting link expiry in the past

Aditionally, in some cases an earlier expiration date was accidentally overwritten by a later one.
This is also now fixed.

https://github.com/cs3org/reva/pull/5346
