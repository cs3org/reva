Bugfix: GetUserByClaim not working with MSAD for claim "userid"

We fixed GetUserByClaim to correctly deal with binary encoded userid
as e.g. used for Active Directory.

https://github.com/cs3org/reva/pull/4251
https://github.com/cs3org/reva/pull/4249
https://github.com/owncloud/ocis/issues/7469
