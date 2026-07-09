Enhancement: Add config option to disable direct (user/group) sharing

Added an `enable_user_sharing` config option to the ocs, usershareprovider
and ocmshareprovider services. When set to `false`, creating user, group
or federated shares is rejected, the `sharees` search endpoint returns
empty results, and the `files_sharing.user.enabled` capability reports
`false`. Public link sharing is unaffected.

https://github.com/owncloud/reva/pull/651
