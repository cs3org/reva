Enhancement: Check permissions before creating shares

The user share provider now checks if the user has sufficient permissions to create a share.
To enable the check, the listGrants permission was added to the viewer and editor roles.

https://github.com/cs3org/reva/pull/4337
https://github.com/cs3org/reva/pull/4340
