Bugfix: allow lightweight accounts to use external apps

For that, we need to explicitly allow all relevant
storage provider requests when checking the lw scope.

https://github.com/cs3org/reva/pull/5444
