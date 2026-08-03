Bugfix: Allow space managers to remove shares created by other users

Space managers with the `RemoveGrant` permission on a resource were unable to
remove user shares created by other users. `Unshare` in the jsoncs3 share
manager only permitted the share creator to remove a share, so removal attempts
by other managers failed with a `NotFound` error, surfaced to callers as a
generic HTTP 500. `Unshare` now also allows users who have the `RemoveGrant`
permission on the shared resource to remove the share, matching the checks
already performed in the `RemoveShare` gRPC handler and `GetShare`.

https://github.com/owncloud/reva/pull/687
