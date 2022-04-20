Bugfix: made uid, gid claims parsing more robust in OIDC auth provider

This fix makes sure the uid and gid claims are defined at init time and that
a proper error is returned in case they would be missing or invalid (i.e. not int64)
when authenticating users.

https://github.com/cs3org/reva/pull/2759
