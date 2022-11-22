Bugfix: OIDC: resolve users with no uid/gid by username

Previously we resolved such users (so called "lightweight" or "external" accounts in the CERN realm)
by email, but it turns out that the same email may have multiple accounts associated to it.

Therefore we now resolve them by username, that is the upn, which is unique.

https://github.com/cs3org/reva/pull/3481
