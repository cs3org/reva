Bugfix: made uid, gid claims parsing more robust in OIDC auth provider

This fix makes sure the uid and gid claims are defined at init time, and that
the necessary typecasts are performed correctly when authenticating users.
A comment was added that in case the uid/gid claims are missing AND that no
mapping takes place, a user entity is returned with uid = gid = 0.

https://github.com/cs3org/reva/pull/2759
