Enhancement: Mint scope-based access tokens for RBAC

Primarily, this PR is meant to introduce the concept of scopes into our tokens.
At the moment, it addresses those cases where we impersonate other users without
allowing the full scope of what the actual user has access to.

A short explanation for how it works for public shares:
- We get the public share using the token provided by the client.
- In the public share, we know the resource ID, so we can add this to the
allowed scope, but not the path.
- However, later OCDav tries to access by path as well. Now this is not allowed
at the moment. However, from the allowed scope, we have the resource ID and
we're allowed to stat that. We stat the resource ID, get the path and if the
path matches the one passed by OCDav, we allow the request to go through.

https://github.com/cs3org/reva/pull/1669
