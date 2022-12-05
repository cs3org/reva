Enhancement: Make max lock cycles configurable

When a file is locked the flock library will retry a given amount of times (with a increasing sleep time inbetween each round)
Until now the max amount of such rounds was hardcoded to `10`. Now it is configurable, falling back to a default of `25`

https://github.com/cs3org/reva/pull/3429
https://github.com/owncloud/ocis/pull/4959
