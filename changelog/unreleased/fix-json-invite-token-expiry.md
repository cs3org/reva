Bugfix: Fix inverted expiry check in JSON invite repository

The `tokenIsExpired` function in the JSON invite repository had the comparison operator inverted, causing valid (non-expired) tokens to be incorrectly filtered out when listing invite tokens.

The check `token.Expiration.Seconds > Now()` was returning true for tokens expiring in the future, effectively hiding all valid tokens. Fixed to use `<` instead of `>`.

https://github.com/cs3org/reva/pull/5418
