Bugfix: Fix public share update

We fixed the permission check for updating public shares. When updating the permissions of a public share while not providing a password, the check must be against the new permissions to take into account that users can opt out only for view permissions.

https://github.com/cs3org/reva/pull/4622