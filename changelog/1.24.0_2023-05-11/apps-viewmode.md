Bugfix: Apps: fixed viewMode resolution

Currently, the viewMode passed on /app/open is taken without validating
the actual user's permissions. This PR fixes this.

https://github.com/cs3org/reva/pull/3805
