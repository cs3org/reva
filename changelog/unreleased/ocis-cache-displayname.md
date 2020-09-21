Bugfix: Cache display names in ocs service

The ocs list shares endpoint may need to fetch the displayname for multiple different users. We are now caching the lookup fo 60 seconds to save redundant RPCs to the users service.

https://github.com/cs3org/reva/pull/1161
