Bugfix: getSpace with spaceID from URL

`getSpace` was getting the first element of path as the ID.
This fix adds a new function `GetIdFromPath`, which uses
the last element of the path as the ID.

https://github.com/cs3org/reva/pull/5150
