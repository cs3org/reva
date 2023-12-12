Bugfix: Allow an empty credentials chain in the auth middleware

When running with ocis, all external http-authentication is handled by the proxy
service. So the reva auth middleware should not try to do any basic or
bearer auth.

https://github.com/cs3org/reva/pull/4241
https://github.com/owncloud/ocis/issues/6692
