Bugfix: list project spaces for share recipients

The sharing handler now uses the ListProvider call on the registry when sharing by reference. Furthermore, the decomposedfs now checks permissions on the root of a space so that a space is listed for users that have access to a space.

https://github.com/cs3org/reva/pull/2419
