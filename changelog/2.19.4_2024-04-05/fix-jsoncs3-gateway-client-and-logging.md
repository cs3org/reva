Bugfix: use gateway selector in jsoncs3

The jsoncs3 user share manager now uses the gateway selector to get a fresh client before making requests and uses the configured logger from the context.

https://github.com/cs3org/reva/pull/4612
