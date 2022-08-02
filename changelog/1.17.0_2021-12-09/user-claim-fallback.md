Enhancement: OIDC: fallback if IDP doesn't provide "preferred_username" claim

Some IDPs don't support the "preferred_username" claim.  Fallback to the
"email" claim in that case.

https://github.com/cs3org/reva/pull/2314
