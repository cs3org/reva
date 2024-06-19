Bugfix: Tolerate missing user space index

We fixed a bug where the spaces for a user were not listed if the user had no space index by user. This happens when a user has the role "User Light" and has been invited to a project space via a group.

https://github.com/cs3org/reva/pull/4710
