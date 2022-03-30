Enhancement: Enable space members to list shares inside the space

If there are shared resources in a space then all members are allowed to see those shares.
The json share manager was enhanced to check if the user is allowed to see a share by checking the grants on a resource.

https://github.com/owncloud/ocis/issues/3370
https://github.com/cs3org/reva/pull/2674
