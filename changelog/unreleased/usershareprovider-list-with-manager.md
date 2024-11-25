Bugfix: Use manager to list shares

When updating a received share the usershareprovider now uses the share manager directly to list received shares instead of going through the gateway again.

https://github.com/cs3org/reva/pull/4971