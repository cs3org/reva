Bugfix: Rename a resource to its current name is a no-op

Renaming a resource to the name it already has failed with an opaque error.
The MOVE handler ran the overwrite check and deleted the existing tree before
noticing that source and destination pointed at the same resource, which could
destroy the resource. The handler now compares the stat'ed resource ids of
source and destination and, when they are equal, returns a silent no-op
(`204 No Content`) before the overwrite and delete logic runs, consistently for
all clients. This also catches id based dav paths where source and destination
references differ but resolve to the same resource; there the same name check
runs before the recursion detection, which would otherwise report the resource
as a child of itself and fail with `409 Conflict`. Users without the move
permission still receive a `403 Forbidden`.

https://github.com/owncloud/reva/pull/708
https://github.com/owncloud/reva/pull/713
https://github.com/owncloud/ocis/issues/1976
