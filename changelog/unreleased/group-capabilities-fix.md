Bugfix: Fix group-based capabilities

The group-based capabilities require an authenticated endpoint, as we must query
the logged-in user's groups to get those. This PR moves them to the `getSelf`
endpoint in the user handler.

https://github.com/cs3org/reva/pull/4400
