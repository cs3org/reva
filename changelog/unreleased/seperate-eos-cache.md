Bugfix: Seperate the eos user and userid cache

Fixes a bug where an invalid value was stored due to the shared cache,
essentially the routine that updates missing values updated an incorrect
key with a UserId instead of a User.

https://github.com/cs3org/reva/pull/5849