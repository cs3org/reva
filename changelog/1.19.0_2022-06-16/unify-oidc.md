Change: Merge oidcmapping auth manager into oidc

The oidcmapping auth manager was created as a separate package to ease testing. As it has now been tested
also as a pure OIDC auth provider without mapping, and as the code is largely refactored, it makes
sense to merge it back so to maintain a single OIDC manager.

https://github.com/cs3org/reva/pull/2561
