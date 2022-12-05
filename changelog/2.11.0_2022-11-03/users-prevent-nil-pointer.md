Bugfix: prevent nil pointer when requesting user

We added additional nil pointer checks in the user and groups providers.

https://github.com/cs3org/reva/pull/3284
https://github.com/owncloud/ocis/issues/4703
