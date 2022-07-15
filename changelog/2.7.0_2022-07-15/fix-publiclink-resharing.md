Bugfix: Forbid resharing with higher permissions

When creating a public link from a viewer share a user was able to set editor permissions on that link.
This was because of a missing check that is added now

https://github.com/owncloud/ocis/issues/4061
https://github.com/owncloud/ocis/issues/3881
https://github.com/owncloud/ocis/pull/4077
