Bugfix: allow scope check to impersonate space owners

The publicshare scope check now fakes a user to mint an access token when impersonating a user of type `SPACE_OWNER` which is used for project spaces. This fixes downloading archives from public link shares in project spaces.

https://github.com/cs3org/reva/pull/3843
https://github.com/owncloud/ocis/issues/5229